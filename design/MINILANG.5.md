# boru Mini-Languages — the `mini` macro and the MiniLang module

**Status:** Phase 1 **LANDED** (2026-06-12) with two kinds: the core
`mini` word (`lang/go/native/native_macro.go`), the `boru:minilang`
module with `re` (Go regexp) and `bf` (brainfuck) plus
`MiniLang.register` / `MiniLang.kinds` (`lang/go/modules/minilang.go`),
static checking through the expansion, and the
`lang/spec/module-minilang.tsv` battery. The rest of the kind
catalogue (§5), compile hooks, and the optional lexer sugar remain
design-only. Rev 2 supersedes rev 1's `xy/` lexer-literal design
(condensed in Appendix A). Core mechanics were first validated with a
pure-boru prototype — every claim marked ✓ ran; the empirical findings
are in §10.

Embedded domain notations — regular expressions, path queries,
transliteration, "natural" infix maths — delivered through one word,
one module, and one standard signature, with **no parser or lexer
changes**.

```
import "boru:minilang"

"AbcD" mini re '[a-z]+'                  # → 'bc'
mini math 'x^2 + 3*y' {x:10, y:2}        # → 106
"2025-03-23" mini re-sub `(\d+)-(\d+)-(\d+)` {repl:'$3.$2.$1'}
                                          # → '23.03.2025'  (backtick src:
                                          #    backslash-safe — see F3)
```

---

## 1. Design in one paragraph

`mini` is a **macro** with surface `mini <kind> <src> <opts?>`. The
kind is a bare word captured as an atom; it names the expansion
target: the call rewrites to the **standard minilang call**

```
mini <kind> <src> <opts>   ⇒   MiniLang.lang_<kind> <src> <opts> end
```

(token chain `MiniLang get lang_<kind> <src> <opts> end`), where
`MiniLang.lang_<kind>` is a word with the **standard minilang
signature** (§3) exported by the `boru:minilang` module. A minilang can
take further inputs from the stack and leave any number of results;
it raises errors through the normal `raise`/`Ideal/Error` machinery.
New minilangs are registered from native Go (§6) or from pure boru
(§7). Because expansion produces an ordinary typed word call, the
static checker sees through `mini` per-kind once expansion is visible
to check mode (§9).

---

## 2. Surface

```
mini <kind> <src>
mini <kind> <src> <opts>
```

| Operand | Capture | Rules |
|---------|---------|-------|
| `kind` | bare word → atom (raw, never evaluated) | **must be literal** — it selects the expansion target at expansion time |
| `src` | one raw form, re-emitted into the expansion | a String literal is canonical. Backtick strings without `${}` are accepted as literals (backslash-safe — see F3). A non-literal form (word, paren group) is allowed and evaluates at the call site, **except** for kinds that declare a compile hook (§11), which require a literal |
| `opts` | one raw Map form, re-emitted | optional; `mini` normalizes a missing opts to `{}`. Values are **not** evaluated at capture — they evaluate in the generated code, at the call site, exactly once ✓ |

`mini` itself is a **core word** (macro category, beside `macro` /
`quote` / `word`): it is language infrastructure and carries no kinds
of its own. All kinds — including the standard library of them — live
in modules and must be registered before first use (define-before-use,
same staging rule as macros).

---

## 3. The standard minilang signature

Every registered minilang `<name>` is exported as the word
`MiniLang.lang_<name>` whose signatures all share the **standard
prefix**:

```
MiniLang.lang_<name> :  [ src:String  opts:Map  …inputs ]  [ …outputs ]
```

- `sig[0]` **src** — the minilang source text.
- `sig[1]` **opts** — named parameters; `{}` when the caller gave none.
- `sig[2..]` **inputs** — kind-declared, zero or more. At a `mini`
  call site these are always filled **from the stack**, because the
  expansion's trailing `end` stops forward collection (§4). ✓
- **outputs** — kind-declared, any number. ✓ (a kind leaving two
  values was exercised in the prototype)

Examples of the two common shapes:

```
MiniLang.lang_re   : [src:String opts:Map subject:String]  [Map]      # filter — input from stack
MiniLang.lang_math : [src:String opts:Map]                 [Number]   # generator — inputs via opts
```

Because the expansion is the standard call, **`mini` is pure sugar**:
the desugared form is always writable by hand and means the same
thing ✓ —

```
"AbcD" mini re '[a-z]+'        ≡        "AbcD" MiniLang.lang_re '[a-z]+' {} end
```

### Why the `lang_` prefix

The kind namespace is **partitioned** inside the one export map:
`mini <kind>` resolves `lang_<kind>` and nothing else. This is safer
than bare kind names and allows **out-of-band exports** — words in
the `MiniLang` namespace that are part of the module's API but are
not minilangs and can never be invoked via `mini`:

```
MiniLang.register   # §7 — register a kind from boru
MiniLang.kinds      # list registered kind atoms (discovery)
MiniLang.lang_re    # a kind — reachable as `mini re …`
```

`mini register …` would look up `lang_register` and fail loudly at
expansion time; conversely a future helper export can never shadow or
be shadowed by a kind. Registration validates the unprefixed name
(lowercase atom, no `lang_` prefix supplied by the caller).

---

## 4. `mini` is a macro — expansion semantics

Expansion uses the landed macro machinery (MACROS.8.md /
legacy/MACROS-PHASE1.10.ignore) and adds no new mechanism:

1. **Raw-capture** `kind` (word, kept un-evaluated), `src` (one form),
   `opts` (raw map; absent → `{}`).
2. **Resolve the target name**: `lang_` + kind atom. If the binding
   does not exist at expansion time, raise
   `[boru/mini_unknown_lang]` with a hint
   (`import "boru:minilang"` / `MiniLang.register`) at the **call
   site** span.
3. **Emit** the token list
   `MiniLang get lang_<kind>  <src-form>  <opts-form>  end`
   and splice it via the existing `__SP` path.
4. **Cache** the expansion under the standard macro key
   (word + operand canon) — identical mini calls expand once. ✓

### The trailing `end` (mandatory, inserted by `mini` itself)

A generated word with unsaturated forward-eligible positions would
otherwise **steal the tokens that follow the mini call** in source.
Demonstrated live (F1): without the `end`,

```
"cAaB" mini re 'Aa' "ZZ"      # match silently ran against "ZZ"; "cAaB" stranded
```

With the `end`, the forward phase stops at the boundary, the
remaining standard-sig positions fill **from the stack** (the §3
inputs contract), and `"ZZ"` passes through untouched ✓. The same
guard makes chained minis bind each subject correctly ✓:

```
"first" mini re 'rs' "second" mini re 'eco'      # two results, no cross-theft ✓
```

This is the engine's own documented idiom — the runaway-collection
error hint already prescribes *"group the call with parens — (… ) —
or end it with `end` or `;`"* — applied mechanically at the one site
where the user cannot apply it by hand (the generated code is
invisible). It is inserted by the `mini` expander, **not** by kind
authors, so it cannot be forgotten. `end` is an ordinary tape value
(`Word/__ED`), so it survives templates, the expansion cache, paren
groups, fn bodies, and `each` bodies ✓ (all exercised).

Consequences, all verified ✓:

- values still cross the boundary via the stack (`(… mini re 'x')
  .ok`, results consumed by later statements);
- a mini call composes inside paren groups, fn bodies, and
  higher-order bodies;
- an un-parenthesized *mixed* enclosing form (`foo a mini re 'x' b`)
  is cut at the `end` — `b` starts a new statement. That shape is
  already `mixed_form_call`-advisory territory; the cut is the
  predictable behaviour, and parens remain the composition tool.

Note for kind authors: kinds must not rely on barrier (`|`)
signatures for theft protection — the auto-`end` owns that. Barriers
remain a per-word design choice for genuinely pipeline-style words,
orthogonal to `mini`.

---

## 5. The `boru:minilang` module

A plain (non-`-util`) module — it is a DSL/framework module per the
naming rule — exporting the `MiniLang` namespace. The standard kinds
are rev 1's catalogue, carried over with the two-letter prefixes
becoming ordinary kind atoms (no lexer involvement):

| Kind atom | Minilang | Standard call shape (after `src opts`) |
|-----------|----------|----------------------------------------|
| `re` | regexp match | `subject:String` → `[Map]` match structure (`{ok ms fst lst n}`) |
| `re-sub` | regexp substitute | `subject:String` → `[String]`; `opts.repl` replacement, `$1…` backrefs, `opts.g` global |
| `re-test` | regexp test | `subject:String` → `[Boolean]` |
| `re-all` | regexp find-all | `subject:String` → `[List]` |
| `tr` | transliterate (Perl `tr///`) | `subject:String` → `[String]`; `opts` flags `d`/`s`/`c` |
| `jp` ✅ | JsonPath (github.com/ohler55/ojg) | `doc:Any` → `[List]` of matched nodes — **landed** |
| `jq` ✅ | jq filter (github.com/itchyny/gojq) | `doc:Any` → `[List]` (the output stream) — **landed** |
| `xp` ✅ | XPath (github.com/antchfx/xpath) | `doc:Xml` → `[List]` of results (elements as Node/Xml values, attribute/text nodes as Strings, a scalar count/string/boolean as a one-element list) — **landed** |
| `cs` | CSS selector | `doc:Any` → `[Any]` (pairs with XML.0.md) |
| `gl` | glob | `path:String` → `[Boolean]` |
| `sh` | POSIX shell pattern | `path:String` → `[Boolean]` |
| `fm` | format template | `args:Any` → `[String]` |
| `ur` | URL pattern | `url:String` → `[Any]` |
| `dt` | date/time format | `text:String` → `[Any]` |
| `m` ✅ | natural infix maths (github.com/tabnas/expr) | *(generator — no stack input)* → `[Number]`; variables from `opts`; boru int/float coercion — **landed** (the rev-1 `math` kind, shipped as `m`) |
| `bf` ✅ | brainfuck | `input:String` → `[String]` (filter), or *(generator)* with `opts.in`; `opts.steps` execution budget — **landed** |

(`re` ✅ is also landed — all matches by default, `{ok ms fst lst n}`
with capture groups `g` per match, `opts.limit`; the other `re-*`
forms remain future kinds. Each match is `{m i e g}`, and `i`/`e` are
**RUNE indices** into the subject — the unit every boru index means, so
they compose with `slice`/`size` directly (NUR047: Go's regexp reports
byte offsets; `reMatchResult` converts them in one incremental pass).)

(Names are atoms, so longer readable names are free where the
two-letter forms are cryptic — `re-sub` rather than rev 1's `rs/`.
Flags move out of slash-delimited positional syntax into `opts`:
`mini re-sub 'a+' {repl:'x', g:true}` replaces `rs/a+/x/g`. This
retires rev 1's per-prefix content-splitting and escaping rules
entirely.)

The `math` kind is the named-params showcase: the handler parses an
infix expression and evaluates it against variables supplied in
`opts` —

```
mini math 'x^2 + 3*y' {x:10, y:2}          # → 106 ✓ (prototype, fixed parse)
mini math 'x^2 + 3*y' {x:(2 add 8), y:w}   # opts values are call-site
                                            # expressions/vars — evaluated
                                            # in the generated code ✓
```

Free variables in `src` that are missing from `opts` (and, advisably,
unused `opts` keys) are loud errors. With a compile hook (§11) the
parse happens once, at expansion time.

### No auto-import

`boru:minilang` is **not** implicitly imported, even though the core
`mini` word is useless without it. Deliberate, for four reasons:

1. **Policy/capability discipline.** Module imports are policy-checked
   (`modules.go::Resolve` consults the host policy; profiles
   allow/deny per module id). An implicit import would acquire a
   capability the program never stated — and every policy profile
   would have to account for `boru:minilang` whether or not the
   program uses `mini`.
2. **The unbound-until-imported contract.** Every module's spec
   battery pins a negative row of the form "`X.word` errors before
   import". Auto-importing one module breaks the uniformity and
   silently reserves the `MiniLang` namespace in every program.
3. **It buys almost nothing.** A `mini` call without the import is a
   loud expansion-time `mini_unknown_lang` whose hint names the one
   missing line. Programs that don't use `mini` pay nothing either
   way; user-registered kinds need their own defining module imported
   regardless, so auto-import could never cover the general case.
4. **Staging stays uniform.** Define-before-use already governs
   macros (interpreter: left-to-right; future compiled mode: imports
   are compile-time effects preceding expansion). `mini` kinds follow
   the same rule with no special case.

Conveniences that do *not* require auto-import: a REPL profile may
preload the module (the REPL already binds things a script must
import), and a project prelude can import it once.

---

## 6. Registering a minilang — native Go

Registration follows the existing native-module sub-registry pattern
(`BuildRandModule` is the model): an inner native per kind, plus an
FnDef wrapper exported under `lang_<name>`. The module centralizes
this in one helper so a kind is a single declaration:

```go
// lang/go/modules/minilang.go

// MiniLangDef declares one minilang kind.
type MiniLangDef struct {
    Name    string              // kind atom, unprefixed: "re", "math"
    Summary string              // one-line help text
    Inputs  []native.FnParam    // stack inputs AFTER the standard [src opts] prefix
    Returns []*native.Type
    Handler native.Handler      // args[0]=src, args[1]=opts, args[2..]=Inputs
}

// AddMiniLang registers def into the module's sub-registry and
// exports the wrapper as lang_<name>. Mirrors wrapRandFnDef:
// trivial-delegation body, BarrierPos -1 (CRITICAL for the swap
// form — see lang/go/CLAUDE.md "Module FnDef Wrappers").
func AddMiniLang(exports *native.OrderedMap, subReg *native.Registry, def MiniLangDef) error {
    if err := validateKindName(def.Name); err != nil { // lowercase atom, no lang_ prefix
        return err
    }
    inner := "minilang-" + def.Name
    params := append([]native.FnParam{
        {Name: "src", Type: native.TString},
        {Name: "opts", Type: native.TMap},
    }, def.Inputs...)
    subReg.RegisterNativeFunc(native.NativeFunc{
        Name: inner,
        Signatures: []native.NativeSig{{
            Args:       fnParamTypes(params),
            Returns:    def.Returns,
            BarrierPos: -1,
            Handler:    def.Handler,
        }},
    })
    exports.Set("lang_"+def.Name, wrapMiniFnDef(inner, params, def.Returns, subReg))
    return nil
}

func BuildMiniLangModule(parent *native.Registry) (native.ModuleDesc, error) {
    subReg, err := native.DefaultRegistry()
    if err != nil {
        return native.ModuleDesc{}, err
    }
    exports := native.NewOrderedMap()

    // ---- kind: re — regexp match ------------------------------------
    if err := AddMiniLang(exports, subReg, MiniLangDef{
        Name:    "re",
        Summary: "regular-expression match: subject from the stack, match map out",
        Inputs:  []native.FnParam{{Name: "subject", Type: native.TString}},
        Returns: []*native.Type{native.TMap},
        Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
            src, err := args[0].AsConcreteString()
            if err != nil {
                return nil, err
            }
            subject, err := args[2].AsConcreteString()
            if err != nil {
                return nil, err
            }
            pat, err := compiledPattern(src) // per-src memo: compile once, reuse (F-perf)
            if err != nil {
                return nil, r.BoruErrorHint("mini_parse_error",
                    fmt.Sprintf("re: %v", err), "lang_re",
                    "fix the pattern; remember '\\d' needs '\\\\d' in quoted strings (or use a backtick string)")
            }
            return []native.Value{matchResultMap(pat, subject, args[1])}, nil
        },
    }); err != nil {
        return native.ModuleDesc{}, err
    }

    // ... lang_re-sub, lang_re-test, lang_tr, lang_math, ... same shape ...

    // ---- out-of-band exports (NOT mini-callable: no lang_ prefix) ----
    exports.Set("register", wrapMiniFnDef("minilang-register", registerParams, nil, subReg)) // §7
    exports.Set("kinds", wrapMiniFnDef("minilang-kinds", nil,
        []*native.Type{native.TList}, subReg))

    return native.ModuleDesc{
        ID:      parent.Modules.NextID(),
        Exports: map[string]*native.OrderedMap{"MiniLang": exports},
    }, nil
}
```

plus the one-line table entry in `lang/go/modules/modules.go`:
`"minilang": BuildMiniLangModule`. `wrapMiniFnDef` is the standard
trivial-delegation wrapper (`Body: [Word(inner)]`, `Registry: subReg`,
`BarrierPos: -1`) — identical in shape to `wrapRandFnDef`
(`rand.go:181`).

Error contract: kind handlers return errors via
`r.BoruError`/`BoruErrorHint` with stable codes (`mini_parse_error`,
`mini_eval_error`, …) carrying `{kind, src, offset}` detail where
applicable, so `do … error […]` handlers can dispatch on `.code` and
the report points at the calling site (legacy/ERRORS.8.ignore §7 quality bar).

---

## 7. Registering a minilang — pure boru

The out-of-band export `MiniLang.register` installs a boru function
as a kind. The function must carry the standard signature prefix —
validated loudly at registration:

```
import "boru:minilang"
import "boru:string-util"

# a counting minilang: src is the substring sought, subject from the stack
MiniLang.register count (fn [
  [src:String opts:Map subject:String] [Integer]
  [ (StringUtil.match src subject {scope:all/q}).n ]
]) end

"banana" mini count 'a'          # → 3 ✓ (verified via the §10 prototype)
"banana" mini count 'z'          # → 0 ✓

# a generator minilang: polynomial over named params (no stack input)
MiniLang.register poly (fn [
  [src:String opts:Map] [Integer]
  [ ((opts.x pow 2) add (3 mul opts.y)) ]
]) end

mini poly 'x^2 + 3*y' {x:10, y:2}        # → 106 ✓ (prototype-proven shape)
```

`MiniLang.register` signature and contract:

```
MiniLang.register : [name:Atom/q  f:Function]  []
```

- `name` — unprefixed kind atom (`/q`: a bare word works). Rejected:
  capitalised names, names already carrying `lang_`, collisions with
  a registered kind (loud `mini_kind_exists`; re-registration of the
  *same* name is an explicit `undef`-then-register, never silent
  shadowing).
- `f` — every signature must start `[String, Map, …]`; otherwise
  `[boru/mini_bad_signature]` with the expected prefix in the hint.
- Effect: installs the function under `lang_<name>` in the `MiniLang`
  export map. Since the F4 engine fix, a raw fn value in an export
  map dispatches like any word (`lang/spec/fn-value.tsv`), so no
  wrapper is *required* for dispatch; `register` is still a native
  word because it owns the **contract** — standard-prefix signature
  validation, kind-name validation, collision checks
  (`mini_kind_exists`), and help/describe metadata. Registration
  stays a word, not "set a key in a map", so those checks cannot be
  bypassed.

Scoping follows module-binding scope: kinds registered in a module
body are visible wherever that module's import is; `undef`-style
teardown on scope exit matches existing binding rules. Two modules
registering the same kind name collide loudly at import time.

### Memoising a boru-registered kind

A built-in kind caches its compiled artifact in a private,
process-global Go map (`re`'s `miniCompiledPattern`, `xp`'s
`miniXPathExprs`) — a table a boru kind author cannot reach, and there
is **no automatic per-kind cache**: the registered `fn` body runs on
every `mini` call. So a boru kind memoises the same way conceptually —
it holds **its own cross-call cache** and short-circuits on a hit. The
cache is a **mutable container** keyed by `src`: a `flex` map (or a
`Store`), either module-level (a dynamic binding the body resolves at
call time) or — cleaner, because it encapsulates the cache with the
kind — **closure-captured** via a factory `fn` (a `flex` map is
pointer-backed, so the captured value shares its state across calls,
the standard factory-retains-state pattern from CLOSURES):

```
import "boru:minilang"

# A factory whose returned fn captures a per-kind cache. Models a kind
# whose `src` carries an expensive "compile" step (here just `add src src`).
def make-dbl fn [[] [Function] [
  def cache (flex {})                                  # captured, mutable
  fn [[src:String opts:Map subject:String] [String] [
    def hit (cache get (src))                          # ← (src), not src
    if (hit is None)
      [ def r (add src src)                            #   the expensive step
        set (src) r cache drop                         # ← (src), not src
        r ]
      [ hit ]                                          #   cache hit
  ]]
]] end

MiniLang.register dbl (make-dbl) end
"z" mini dbl 'aa'        # computes, caches → 'aaaa'
"z" mini dbl 'aa'        # cache hit        → 'aaaa'
"z" mini dbl 'bb'        # computes, caches → 'bbbb'
```

**The one non-obvious rule:** `get` / `set` (and the `m.k` dot form)
**quote their key** (the `/q` slot, like `def`), so a *variable* key
must be parenthesised to evaluate it — `get (src)` / `set (src) v m`.
A bare `get src` keys on the literal atom `src`, so every entry
collides on one key and every lookup falsely hits. A *constant* key
needs no parens (`set 'n' …`, `cache.size`).

The standard-library spec pins this: `lang/spec/module-minilang.tsv §3`
registers a memoising `dbl` and asserts the body computes exactly
**twice** for three calls across two distinct `src` (the repeat is a
hit). A module-level `def cache (flex {})` referenced from the body
works identically — the factory just keeps the cache out of the
program namespace.

Note the contrast with the built-ins: a boru kind's cache is **per
kind-instance / per engine**, not process-global, so it needs no mutex
(one engine steps single-threaded) and never leaks across `lang.New()`
instances — the inverse trade-off to the Go memos, which take a mutex
to share one table across every instance. The compile hook (§13,
`register-compiled`) is a *staging* mechanism — it splices precomputed
tokens at the call site, materialising "compile once" only on the
bytecode-compiled path — not a runtime cache; for interpreter-side
memoisation the captured-`flex`-map pattern above is the route.

---

## 8. Errors

| Stage | Error | Notes |
|-------|-------|-------|
| expansion | `mini_unknown_lang` | kind not registered; hint suggests the import / `MiniLang.kinds`; span at the call site |
| expansion | `macro_error` (arity) | missing `src`; standard macro operand error ✓ |
| registration | `mini_bad_signature`, `mini_kind_exists`, `mini_bad_name` | §7 contract |
| runtime | `mini_parse_error` | malformed `src` for the kind's grammar, with `{kind, src, offset}` payload |
| runtime | kind-specific raises | ordinary `raise`/`Ideal/Error` values; `.code`-dispatchable; catchable with `do [...] error [...]` ✓ |

Everything is loud; there is no generic-fallback path (contrast rev
1's "unrecognized prefixes produce a generic MiniLang value", which
turned typos into values — F-rev1).

---

## 9. Static checking and discovery

Post-expansion, a mini call **is** an ordinary wrapper-word call, so
`boru check` validates per call site against the kind's standard
signature — `12345 mini re 'a+'` is a static signature error on
`lang_re`'s `subject:String`, and outputs type the surrounding
program per kind.

Two facts gate / support this (verified):

- check mode does **not** yet expand user-level `macro` definitions
  (F5) — `mini` as a *native* macro must take the native splice path,
  which the checker already follows: a `word`-spliced body containing
  a wrapper call **and the trailing `end`** checks clean with types
  flowing through ✓. So implementing `mini` natively (it must be
  native anyway, for the optional-opts arity and call-site error
  spans) delivers static checking without waiting on check-mode macro
  expansion; landing the latter then also covers boru-prototyped
  macros generally.
- discovery: `describe mini` lists the registered kind atoms with
  each kind's effective signature (the standard call including its
  stack inputs and outputs) — `mini`'s own signature deliberately
  says nothing about stack effect, so the per-kind listing is the
  documentation. `boru describe boru:minilang` lists the exports;
  `boru describe boru:minilang:lang_re` documents one kind;
  `MiniLang.kinds` answers from code.

---

## 10. Validated by prototype (2026-06-12)

A pure-boru prototype of the full rev 2 pipeline ran against the live
engine (built from this branch; source in Appendix B): a `mini` macro
that raw-captures `kind`/`src`/`opts`, builds `lang_<kind>` from the
kind atom at expansion time, and splices
`MiniLang get lang_<kind> <src> <opts> end` — dispatching through a
real `import module [export "MiniLang" {…}]` (post-F4-fix; earlier
runs used flat `def lang_re …` stand-ins) — with standard-signature
kinds `lang_re` (filter — subject from stack, backed by
`StringUtil.match` literal matching) and `lang_poly` (generator —
inputs from `opts`). One scoping fact the module form surfaces: the
kind body runs in **module scope**, so its dependencies are imported
inside the module body, not inherited from the caller (pinned in
`fn-value.tsv` §3).

Proven ✓: kind→word dispatch by name atom; subject-from-stack through
the standard signature (infix form); opts values as literals,
call-site expressions, and variables (deferred evaluation, evaluated
once in the generated code); multi-value kinds; `macroexpand`
introspection of mini calls; expansion caching; loud unknown-kind and
arity errors; auto-`end` in paren groups / fn bodies / `each` bodies /
chained mini calls; equivalence of the sugared and desugared forms;
check-mode inference through an `end`-carrying native splice.

Empirical findings the design must respect:

- **F1 — token theft.** Without the trailing `end`, the generated
  word forward-collects the *next source token* as its subject
  (silent wrong result). The auto-`end` closes it; spec rows must pin
  both directions (`"x" mini re 'p' "y"` leaves `"y"`; chained minis
  bind their own subjects).
- **F2 — argument order in generated code.** Binary non-commutative
  handlers compute `args[1] op args[0]`; a kind compiler emitting boru
  (the `math` kind) must emit infix / fully-parenthesized code —
  `pow 10 2` is 2¹⁰, `10 pow 2` is 10². Every code-generating kind's
  battery needs a non-commutative case.
- **F3 — string escaping.** Both `'…'` and `"…"` process backslashes
  (`'\d+'` → `d+`): regex sources need `'\\d+'` or an
  interpolation-free backtick string (backslash-raw, verified). The
  `re` family's docs and `mini_parse_error` hint must say so.
- **F4 — raw `fn` values didn't forward-dispatch as values — FIXED
  (engine), pinned by `lang/spec/fn-value.tsv`.** As found: with
  `def m {f: (fn [[a:Integer][Integer][a add 1]])}`, the call
  `m.f 5` left `fn …` and `5` as silent residue (while an `afn`/`=>`
  lambda dispatched), and a `module [export …]` fn export was inert.
  Mechanism: `m.f 5` is the token chain `m get f 5`; the `get` result
  dispatches via `execFnDefLiteral`, where (a) a matched handler-less
  non-anonymous sig was sent to the legacy *pure-stack* FnSig path
  even when the match included forward positions, and (b) an unnamed
  FnDef carrying a foreign sub-registry (an anonymous fn in a module
  export map) had nothing to name-look-up and went inert before
  matching at all. The fix (engine.go::execFnDefLiteral): a matched
  handler-less sig with forward positions now proceeds to the
  existing `insertForward` parking branch (which re-processes the
  value with all args on the stack, landing in the legacy stack path
  that already dispatched correctly), and an unnamed foreign-registry
  FnDef falls back to compiling its own authored sigs — its body
  still runs in module scope via `fnDef.Registry`. Macro values stay
  data (unchanged), bare/unmatched values stay data (unchanged,
  pinned), and an outer import still does not leak into a module
  body (pinned). Consequence for this design: a plain
  `module [export "MiniLang" {lang_re: (fn …)}]` works directly —
  the §B prototype runs the real `MiniLang get lang_<kind>` chain.
  `MiniLang.register` remains a native word for the *contract*
  (standard-prefix validation, collision checks, help metadata), no
  longer out of dispatch necessity.
- **F5 — check-mode macro gap.** `boru check` does not expand
  user-level macros (a `def m (macro …)` use reports
  `undefined_word`). Native splice integrates today (see §9); native
  `mini` is the route to static checking now, check-mode macro
  expansion is the follow-on that generalizes it.
- **F6 — Word values have no data form; canon scope.** `canon`'s
  round-trip guarantee is scoped to **data** values (canon.tsv §1–4:
  scalars, atoms, paths, none, type literals, Node trees; identity
  values render but rebuild fresh). A raw-captured **Word** value is
  not source-expressible at all — bare `re` re-parses as an
  invocation and `re/q` as an Atom — so canon renders it as the
  deliberately non-syntax marker `word(re)` (the same rendering the
  macro.tsv goldens pin). Consequence: pure-boru name-constructing
  macros currently have no clean route from a captured Word to its
  name (the prototype stripped the `word(…)` wrapper — a stopgap;
  `quote`/`inspect`/`convert` don't reach it). The native `mini`
  reads `WordInfo.Name` directly and never touches this.
  **Remedy (recommended, not a dependency):** extend `convert`
  (`lang/go/native/native_type.go:254`) with two Word-source sigs —

  ```go
  // Word → Atom / String: the data forms of a code identifier.
  // Completes the existing round trip: an Atom spliced via unquote
  // already becomes a Word (the gensym mechanism); this is the
  // inverse, and the Atom form canon-renders round-trippably (re/q).
  {Args: []*Type{TAtom, TWord},   TypeArgs: map[int]bool{0: true},
   Handler: convertWordHandler, Returns: []*Type{TAtom},   BarrierPos: -1},
  {Args: []*Type{TString, TWord}, TypeArgs: map[int]bool{0: true},
   Handler: convertWordHandler, Returns: []*Type{TString}, BarrierPos: -1},
  ```

  after which a pure-boru `mini` reads cleanly
  (`def wn (convert Atom ("lang_" add (convert String kind)))`).
  `convert` is the right home (the established cross-type gateway
  with TypeArgs target dispatch); overloading `quote` would conflate
  token capture with binding reads, and `inspect` is diagnostics.
  Pair the negative: `convert Integer <word>` stays a signature
  error.

---

## 11. Phasing

| Phase | Deliverable |
|-------|-------------|
| 1 | **LANDED 2026-06-12** (scoped to two kinds): native `mini` (two sigs `[Atom/q String]` / `[Atom/q String Map]`; `lang_` resolution with expansion-time `mini_unknown_lang`; auto-`end`; opts normalized to `{}`; `RunInCheckMode` so the checker steps the expansion) + `boru:minilang` with `re` (Go `regexp`, per-src compile memo) and `bf` (brainfuck — filter + generator forms, `opts.steps` budget) + `MiniLang.register` / `MiniLang.kinds` + battery `lang/spec/module-minilang.tsv`. Implementation notes: `mini` returns an `__SP` splice of the standard-call tokens (the `word` mechanism) rather than going through the macro expander — so there is no expansion cache (the `re` compile memo covers the hot cost) and `macroexpand` does not apply to `mini`; src is spliced as collected, so dynamic src works for runtime kinds. Deferred from the original Phase-1 row: `re-sub` / `re-test` / `re-all` |
| 2 | **`m`** ✅ (Pratt maths via github.com/tabnas/expr — the rev-1 `math` kind, shipped as `m`), **`jp`** ✅ (JSONPath via github.com/ohler55/ojg), **`jq`** ✅ (jq via github.com/itchyny/gojq); remaining: `tr`, `fm`, `gl`. `jp`/`jq` take any boru document — a Node (Map/List), Object, Array, Table or Record — converting it to generic data (Ideals project through their IdealConverter) and the matches back to boru values; both return a List of results (a Map/Record subject needs an explicit `{}` opts, the same gotcha as `gex`) |
| 3 | **compile hooks**: a kind may register an expansion-time compiler `(src, opts-form) → token list` that `mini` splices *instead of* the standard call — staged compilation of the DSL (parse once ever, splice precompiled carrier values, surface `src` syntax errors at expansion time with call-site spans; requires literal `src`). The standard call remains the semantic reference and the dynamic-src fallback |
| 4 | **`xp`** ✅ (XPath via github.com/antchfx/xpath — queries a Node/Xml document, the stack subject, now that XML is a node type; a navigable mirror tree feeds the antchfx cursor model); remaining catalogue kinds (`cs`, `ur`, `dt`, `sh`); the `+` literal shortcut — **LANDED** (§12) |

---

## 12. The `+name<delim>src<delim>` shortcut — LANDED

The terse literal form rev 1 wanted, recovered as **pure lexer sugar over
`mini`** (the Phase-4 option) rather than rev 1's whitespace-terminated
two-letter prefixes (Appendix A). A custom jsonic LexMatcher
(`eng/go/parser/grammar.go::setupMiniLitMatcher`) recognises

```
+<name><delim><src><delim>
```

and the converter (`parse.go`) emits the **identical token stream** as the
explicit call — `mini <name> '<src>'` — so every property of `mini` carries
over for free: a stack subject, a trailing `{opts}` map (collected by mini's
3-arg sig), the expansion-time `mini_unknown_lang` error, and check mode.

- **Delimiter** is the first character after the (lowercase) `name`; the
  **same** character closes. Pick any non-space char, so a source containing
  one delimiter just uses another: `+re|a/b|`, `+re#a/b#`.
- **Backslash** escapes the delimiter, itself, and whitespace (`\<delim>` →
  the delimiter, `\\` → `\`, `\<space>`/`\<tab>` → the space); every **other**
  backslash is preserved **raw**. So regex keeps its escapes without doubling
  — `+re/\d+/` is the pattern `\d+`, versus the quoted form's `'\\d+'` (the
  F3 escaping wart, gone for this surface).
- **Trigger** is `+` immediately followed by a lowercase letter, a non-space
  delimiter, and at least one source char. `+0d5` (a signed bignum) and a bare
  `+` are left to normal lexing, as is an empty unterminated source (`+re:`
  before whitespace).
- **A literal is a single whitespace-free span** — its source never contains
  unescaped whitespace, and the first unescaped delimiter in the span closes
  it (so a closed literal can abut syntax: `+re/x/).fst`). **The closing
  delimiter is optional** when the source does not contain the delimiter
  char: with no delimiter before the first whitespace (or end of source), the
  span IS the source — `+email:alice@example.com` ≡
  `mini email 'alice@example.com'`. There is no scan-ahead past whitespace,
  so a stray delimiter char in a later token (the `:` in `{limit:2}` or
  `{limit: 2}`) can never capture the literal, and a literal never spans
  lines. A source that needs whitespace escapes it (`\<space>` — see
  Backslash above) or uses the quoted `mini` form; hb/bb sources group with
  `_` (`+hb/de_ad_be_ef/`).
- **Sugar only, opts-less in the literal** — for options, the trailing map
  rides the desugared call (`+re/\d/ {limit:2}`). There is no first-class
  compiled-pattern *value* yet; that remains the Phase-3 compile-hook /
  `boru:regexp`-style story.

Examples (all equivalent to the explicit `mini` call):

```
"AbcD" +re/[a-z]+/                 ≡  "AbcD" mini re '[a-z]+'
("a1b2c3" +re/\d/ {limit:2}).n     ≡  ("a1b2c3" mini re '\\d' {limit:2}).n   # → 2
+bf/++++++++[>++++++++<-]>+./       ≡  mini bf '++++++++[>++++++++<-]>+.'      # → 'A'
```

Implementation: matcher → `#ML` token carrying `miniLitVal{Name,Src}` →
val-rule Open alternate sets `r.Node` → `convertTopLevelItems` emits the three
tokens inline in word context (data-context map values fall back to an `__SP`
splice in `convertTopLevelValueInner`). Battery: `lang/spec/module-minilang.tsv`
§6 + `lang/go/test/minilang_literal_test.go`.

---

## 13. Compile hooks — LANDED (additive; the transducer stays)

The Phase-3 staged-compilation idea, delivered **additively**: a kind keeps
its runtime transducer and may register an OPTIONAL expansion-time **compile
hook**. When `mini <kind> <src>` is stepped and `src` is concrete, the hook
runs at the call site and returns the tokens `mini` splices instead of the
standard `MiniLang.lang_<kind> src opts end` call.

The transducer is **load-bearing**, not legacy: it is the semantic reference,
the **check-mode target** (the checker always validates the standard call —
compile hooks never run in check mode), and the **fallback** when `src` is not
concrete. So the immediate-transduction definition API is unchanged; compile
hooks are layered on top.

**One registration path, one discovery point** (`miniHandler`):

- **Go** — a built-in's `native.RegisterMiniCompileGoHook`. The hook is a Go
  func `(src, opts, r) → []Value`. Stored in a per-registry table.

> **Retired (2026-07):** the boru path — `MiniLang.register-compiled <name>
> (macro …)` stored as `compile_<name>` — died with the frozen kind
> namespace (`register-compiled` is a tombstone raising
> `mini_registry_frozen`; an expansion-time macro rewrite is
> unrepresentable as a runtime value, so it has no value-form successor).
> The former `MiniLangSpec.Compile` field went with it: hooks are Go-only
> BUILT-IN machinery now, and a custom fn-value mini-language memoizes any
> expensive compile inside its own body.

**Built-in demo — `re` (carrier + consumer):** `miniReCompile` compiles the
pattern at the call site (memoized), wraps the `*regexp.Regexp` in an inert
`Ideal/MiniLangCompiled` carrier (FixedID 5003), and splices
`MiniLang get run-re <carrier> <opts> end`. `run-re` matches the precompiled
pattern against the stack subject — per-call cost is just the match. A
**malformed pattern defers to the standard `lang_re` call** rather than raising
early: the carrier can't be built, and deferring keeps the error byte-identical
to the transducer — which is what the bytecode VM runs (the recorder, in check
mode, never takes the compile-hook path), so compiled/interpreted parity holds
(`TestSpecCompiledOrFallback`). This is the general rule for hooks: a compiled
splice must be observationally identical to the transducer, errors included.

**Two architectural facts that shaped the scope** (and ruled out a
literal-only / strict mode):

1. `mini` expands at **execution** time, not parse time, so `src` is already
   evaluated — a variable is indistinguishable from a string literal. There is
   no way to reject "runtime-computed src" without raw-capturing `src` (a
   deeper change to the core word's collection + the dynamic-src splice).
2. `mini` has **no expansion cache** (Phase 1: "the `re` compile memo covers
   the hot cost"), so even a literal recompiles every time the call is stepped.
   Literal-gating buys no "compile once" win; hooks **memoize** instead (as
   `re` does via `miniCompiledPattern`).

So the contract is simply: *the hook runs whenever `src` is concrete; check
mode and a non-concrete src use the transducer.* A hook author guarantees the
compiled tokens are semantically equivalent to the transducer (the
differential reference).

Battery: `lang/spec/module-minilang.tsv` §7 + `lang/go/test/minilang_compile_test.go`.

---

## Appendix A — rev 1 (superseded): `xy/` lexer literals

Rev 1 proposed two-letter whitespace-terminated lexer literals
(`rm/a+/`, `rs/foo/bar/g`, `jp/$.store.book`, …): a custom jsonic
LexMatcher, a `Scalar/MiniLang` value hierarchy carrying
prefix/content, per-prefix content parsers (slash-splitting with
`\/` escapes), and a Go-side prefix registry, with dispatch through
suffix-position signature matching.

Dropped because the `mini` macro keeps the registry idea while
deleting the delivery costs:

- **whitespace termination** forbade spaces in patterns; `src` is now
  an ordinary string literal;
- **per-prefix splitting/escaping rules** (`rs/p/r/flags`, `\/`) are
  replaced by one string plus an `opts` map;
- **two-letter prefix squat** and collision rules with two-letter
  words disappear — kinds are ordinary atoms;
- **silent generic fallback** ("unrecognized prefixes produce a
  generic MiniLang value") turned typos into values; unknown kinds
  now error at expansion time;
- **Go-only extensibility** (lexer matcher + Go registry) becomes
  registration from boru itself (§7);
- **a new lexer matcher and value hierarchy** versus zero new
  syntax: rev 2 rides the landed macro machinery.

What rev 1 had that rev 2 gives up: terseness (`rm/a+/` vs
`mini re 'a+'`) and first-class pattern literals (`set pat rm/\d+/`).
Both are recoverable later — the literal syntax can return as pure
sugar that desugars to `mini` (Phase 4), and first-class compiled
values arrive with compile hooks (Phase 3) or per-domain modules
(e.g. a future full `boru:regexp` with a `compile → instance` API in
the `Rand.with-seed` style). Rev 1's kind catalogue is retained as
§5's standard library.

---

## Appendix B — the validated pure-boru prototype

The §10 prototype, verbatim as last run (green). This is *model*
code: the real `mini` is native (`lang/go/native/native_macro.go`,
beside the macro words — **landed**) and the standard kinds are Go
(`lang/go/modules/minilang.go` — **landed**); the executable spec is
now `lang/spec/module-minilang.tsv`. The appendix remains as the
record of the design-validation prototype.

```
# Pure-boru prototype of the rev-2 minilang design (validated 2026-06-12).
# Models the native mini exactly, except the F6 name-construction wart
# (the canon strip below; native mini reads WordInfo.Name).

import "boru:string-util"

# --- the MiniLang module: kinds carry the STANDARD signature
#     [src:String opts:Map ...inputs] [...outputs], exported as lang_<name>.
#     NB: kind bodies run in MODULE scope — import dependencies here.
import module [
  import "boru:string-util"
  export "MiniLang" {
    lang_re:   (fn [[src:String opts:Map subject:String] [Map]
                   [ StringUtil.match src subject ]]),
    lang_poly: (fn [[src:String opts:Map] [Integer]
                   [ ((opts.x pow 2) add (3 mul opts.y)) ]])
  }
]

# --- the mini macro: kind atom -> MiniLang.lang_<kind>, auto-`end`.
def mini (macro [[kind src opts] [
  def kn (StringUtil.replace ')' '' (StringUtil.replace 'word(' '' (canon kind))) end
  def wn (convert Atom ("lang_" add kn)) end
  quote [ MiniLang get unquote wn unquote src unquote opts end ]
]]) end

def r ("AbcD" mini re 'bc' {}) end
print r.fst end                              # {m:'bc' i:1 e:3}
print (mini poly 'x^2 + 3*y' {x:10, y:2}) end   # 106
"cAaB" mini re 'Aa' {} "ZZ" end              # theft guard: ZZ untouched
print (macroexpand (mini re 'bc' {})) end    # [MiniLang get lang_re "bc" {} end]
```

Differences from the real thing, all noted in the body: fixed
3-arity (native `mini` has the 2-sig optional-`opts` surface and
normalizes `{}`), the F6 canon strip for the kind name, if/registry
dispatch done by the module export map directly (native adds the
expansion-time `mini_unknown_lang` check with a call-site span), and
`StringUtil.match` literal matching standing in for a real regexp
backend.
