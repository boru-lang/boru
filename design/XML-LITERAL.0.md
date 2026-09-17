# boru Literal XML Embedding Design

Status: Increments 1–5 **LANDED**. Increment 1: static (non-interpolated)
`Node/Xml` literals parse end-to-end, build an immutable element value,
render back to well-formed XML, and carry structural equality. Increment
2: `${}` interpolation (§4) — in text, child, and attribute positions.
Increment 3: the mutable `Node/Xml/FlexXml` variant via `flex`/`node`,
with in-place `append` (children) and `set` (attributes). Increment 4:
`parse xml` now yields a `Node/Xml` element (not a generic Map), so a
parsed document and a source literal are deeply equal (§5.6). Increment
5: the accessor/query words — `tag`/`attr`/`cren` via dotted access, plus
the computed `xml-elem` / `xml-text` / `xml-attr` (all `xml-` prefixed so
they never shadow the generic `text`/`elem` identifiers). The `cs/`
CSS-selector surface (§4) remains a follow-up — it rides the
(as-yet-unlanded) `cs` mini-language kind.

Implementation landed in Increment 1:
- type `Node/Xml` (FixedID 108) + `XmlElementPayload{Tag,Attr,Cren}` +
  `xmlBehavior` (Format → XML, structural Equal): `eng/go/core_xml.go`,
  `eng/go/payload.go`, `eng/go/value.go` (`NewXmlElement`/`AsXmlElement`),
  `eng/go/typetable.go`, `eng/go/types.go`.
- grammar: a `val.Open` `<` alternate pushes an `xml` rule that arms the
  `xml_literal` lex matcher; the matcher scans the whole balanced element
  and builds the value directly: `eng/go/parser/xml_literal.go`,
  `eng/go/parser/grammar.go`, `eng/go/parser/parse.go`.
- batteries: `eng/spec/xml-literal.tsv`,
  `eng/go/parser/xml_literal_test.go`, `lang/go/test/xml_literal_test.go`.

Implementation landed in Increment 2 (interpolation):
- a literal embedding any `${expr}` builds a deferred skeleton instead of
  a constant — type `Word/__XI` (FixedID 109) + `XmlInterpPayload`/`XmlTmpl`
  (`eng/go/payload.go`, `eng/go/types.go`, `eng/go/typetable.go`,
  `eng/go/value.go` `NewXmlInterp`/`AsXmlInterp`/`IsXmlInterp`/`IsXmlElement`).
- the lex matcher builds an `XmlTmpl` and *freezes* it to a constant
  `Node/Xml` when no hole is present, else emits the skeleton; `${…}` is
  extracted by brace-depth scan (quote-aware) and parsed via `Parse`
  (`eng/go/parser/xml_literal.go`).
- the engine evaluates the skeleton in place to a `Node/Xml` —
  `evalXmlInterp`/`buildXmlFromTmpl`, mirroring `evalInterpString` and
  dispatched at the same four sites (main loop, forward-collection,
  paren-eval, map-value). The child splice rule: a List contributes each
  element, a `Node/Xml` is one child, any other value becomes a text node;
  adjacent text merges (`eng/go/engine.go`).
- batteries: interpolation rows in `eng/spec/xml-literal.tsv`,
  `TestXmlLiteralFreezeVsInterp`/`TestXmlLiteralInterpErrors`
  (`eng/go/parser/xml_literal_test.go`), `TestXmlLiteralInterpolation`
  (`lang/go/test/xml_literal_test.go`).

Implementation landed in Increment 3 (mutable FlexXml):
- type `Node/Xml/FlexXml` (FixedID 110, child of Node/Xml so it inherits
  the Xml Behavior) + pointer-backed `*FlexXmlData` payload
  (`eng/go/types.go`, `eng/go/typetable.go`, `eng/go/payload.go`,
  `eng/go/value.go` `NewFlexXml`/`AsFlexXml`, `eng/go/util.go` `IsFlexXml`).
- `xmlBehavior` now reads either payload through a shared `xmlParts`
  accessor, so Format/Equal treat immutable and flex elements uniformly
  (`eng/go/core_xml.go`).
- the kernel flex primitives gained an Xml branch: `FlexDeepCopy`,
  `NodeDeepCopy`, `containsFlex`, `AdoptIntoFlex`, and `MakeNodeHandler`
  (`make FlexXml`/`make Xml`) — a flex copy is deep and never aliases the
  immutable source (`eng/go/core_flex.go`).
- words: `flex`/`node` round-trip XML unchanged; `append <child> f` (and a
  List splice) grow children in place; `set <name> <val> f` sets an
  attribute (DOM setAttribute) (`lang/go/native/native_flex.go`,
  `native_storage.go`, aliases in `native/aliases.go`).
- batteries: `TestXmlFlexEndToEnd` (`lang/go/test/xml_literal_test.go`),
  `TestFlexXmlRoundTrip` (`eng/go/flex_xml_test.go`), Node/Xml/FlexXml in
  the FixedID stability snapshot.

Implementation landed in Increment 4 (`parse xml` alignment):
- the tabnas `parse xml` handler now converts its `{name, attributes,
  children}` output into a `Node/Xml` element via `tabnasXmlToValue`
  rather than a generic Map — text children stay Strings, element
  children recurse, attributes sort for determinism
  (`lang/go/modules/parselang.go`; `Returns` is now `TXml`; doc updated in
  `docs_parselang.go`). `NewXmlElement` re-exported in `native/aliases.go`.
- `DeepEqual` gained an XML branch routing to `xmlElementsEqual`
  (`eng/go/compare.go`), so `deq` is true for a parsed element and the
  equivalent literal (and for a flex copy and its source); attribute
  order is not significant.
- batteries: `TestXmlParseAlignment` (`lang/go/test/xml_literal_test.go`),
  updated rows in `lang/go/test/parselang_tabnas_test.go` and
  `lang/spec/module-parselang.tsv`.

Implementation landed in Increment 5 (accessor / query words):
- `get` reads the well-known fields of a Node/Xml (or FlexXml): `'tag'` →
  String, `'attr'` → Map, `'cren'` → List, any other key → None. Sigs are
  `[Atom|Xml]`/`[String|Xml]`, more specific than `[Key|Node]` so they win
  for an XML receiver — dotted access `x.tag` / `x.attr` / `x.cren` works
  (`lang/go/native/native_storage.go::getXmlHandler`).
- computed words in `lang/go/native/native_xml.go` (`xmlNatives`, wired in
  `register.go`): `xml-elem` (element children only), `xml-text` (subtree
  text), `xml-attr <name> <xml>` (attribute value or None). `eng.XmlParts`
  is the exported (tag, attr, cren) view they share (`eng/go/core_xml.go`,
  aliased in `native/aliases.go`).
- batteries: `TestXmlAccessors` (`lang/go/test/xml_literal_test.go`); the
  fnmodel equivalence golden regenerated (3 new words + 2 get sigs).
- not done: the `cs/` CSS-selector words (`xml-is`, selector engine) —
  deferred with the `cs` mini-language kind.

Literal XML embedded directly in boru source — `<tag …>…</tag>` written
in-line where a value is expected — producing a first-class `Node/Xml`
value. This is the JSX/TSX analogue for boru: structural XML data
written in source the same way jsonic `{…}` / `[…]` literals already
embed structural data today.

> **Not to be confused with `design/XML.0.md`.** That document
> describes the inverse direction — encoding an entire boru *program* as
> an XML tree (`<boru><def>…</def></boru>`) for tooling and code
> generation. This document is about embedding XML *data values* into
> ordinary concatenative source. The two share the `${}` interpolation
> convention (§4) and the CSS-selector query surface (§5.5) but are
> otherwise independent features. Where they touch — the `Node/Xml`
> value an embedded literal yields is exactly what a program-XML
> `<boru-embed lang="xml">` block would also yield — is called out in §7.


## 1. Motivation — the JSON analogy, and why it is not quite JSON

boru has **no** `json "…"` literal form. Its relaxed-JSON jsonic
literals — `{a:1, b:[2 3]}`, `[x y z]` — *are* the embedded
structured-data mechanism: you write the data tree directly in source
and the parser turns it into `Node/Map` / `Node/List` values
(`eng/go/parser/parse.go::convertMapData` / `convertDataList`). There
is no separate "parse a JSON string at runtime" step for source-level
data; runtime parsing exists (`parse json '…'`,
`StructUtil.parse`) but it is for *external* text, not for literals a
programmer types.

XML has only the runtime-parse half today: `parse xml '<r><a>1</a></r>'`
(`lang/go/modules/parselang.go:414`, the tabnas `Xml` plugin) takes a
**string** and returns a plain `Node/Map`. There is no way to write the
tree *literally*. This document closes that gap, making literal XML the
first **tagged-tree** literal in boru — beyond the untagged map/list
trees jsonic already gives us.

The model is JSX/TSX: in React you write `<div className="x">{kids}</div>`
inline in JavaScript and it compiles to `createElement` calls. boru's
concatenative surface is even more amenable — a literal is just another
value on the tape — so the literal can evaluate to a `Node/Xml` directly
with no framework runtime.

**Scope.** This is about XML used as *data* (documents, config,
markup, feeds, SVG, HTML fragments). It is deliberately *not* a second
concrete syntax for boru programs — that is `XML.0.md`'s job.


## 2. Surface syntax

An XML literal is a value. It appears anywhere a value is expected — a
binding body, a word argument, a list/map element, a paren group:

```
def page <html><body><h1>Title</h1></body></html>

[ <li>a</li> <li>b</li> <li>c</li> ]

print (text <p>hello world</p>)
```

Element forms:

| Form | Example | Meaning |
|------|---------|---------|
| element | `<tag a="1">…</tag>` | open tag, children, matching close |
| self-closing | `<br/>` , `<img src="x"/>` | no children |
| fragment | `<>…</>` | child list with no wrapping element (optional — §7) |
| attributes | `<a href="u" rel="x">` | `name="value"` pairs; quoted values |
| comment | `<!-- … -->` | preserved or dropped per policy (§7) |

Attribute values are double- or single-quoted strings, or an
interpolation (`§4`). Boolean/valueless attributes (`<input disabled>`)
are admitted as `disabled=""` in strict mode (HTML-ish bare attributes
are a FlexXml concern — §5.2).

Whitespace between elements is insignificant in element content and
significant inside text runs; the exact normalisation policy is an open
question (§7).


## 3. Disambiguation versus generic type sugar (the crux)

`<` and `>` are already meaningful in boru: they are the **generic type
sugar** delimiters, `Box<Integer>` ≡ `(Box of [Integer])`
(`design/legacy/GENERICS.10.ignore` D14/D15). Adding XML must not disturb that.
It does not — and the reason is structural, not a heuristic.

### 3.1 How angle brackets work today

`<` (`#LA`) and `>` (`#RA`) are registered as standalone fixed jsonic
tokens in `eng/go/parser/grammar.go::setupBaseTokens`, independent of
generics. Decision D14 made this explicit so that *future features can
register additional grammar consumers without re-lexing* — XML
embedding is named there as the motivating example.

Generics consume `<` through a **`val.Close` alternate** that is
**gated** (`setupAngleGrammar`, grammar.go ~1335):

```go
// gate: the just-closed val is a capitalised bare name
angleGate := func(r, ctx) bool {
    txt, ok := r.Node.(jsonic.Text)
    if !ok || txt.Quote != "" || txt.Str == "" { return false }
    c := txt.Str[0]
    return c >= 'A' && c <= 'Z'         // capitalised name only
}
```

So `<` opens a generic angle group **only** when the value that *just
closed* is a capitalised bare name — the type-name convention. A `<`
anywhere else "matches no rule and is a syntax error in v1, only because
no other consumer exists yet" (the rule's own doc comment). That dangling
syntax-error slot is the hook XML fills.

### 3.2 The non-colliding hook: a `val.Open` alternate

Generics live in the **close (suffix) position** — `<` *after* a complete
capitalised value. XML lives in the **open (value) position** — `<` where
the parser expects a *fresh value* to begin. These are different states
of the same `valof` rule, so the two consumers never compete for the same
`<`:

- Add `<` as a **`val.Open` alternate** that opens a new `xml` rule,
  with a one-token lookahead predicate: the next token after `<` must be
  a name-start character (a tag) or `/` (a closing tag / fragment).
- A bare `<` followed by whitespace or an operator is left alone (still a
  syntax error, as today — `lt`/`gt` are the comparison words, never
  `<`/`>`).

Worked traces:

```
def x Box<Integer>        # `Box` closes → val.Close sees `<` → GENERICS
def x <div>hi</div>       # value position → val.Open sees `<div` → XML
[ <li>a</li> ]            # list element is value position → XML
f <p>x</p>                # word arg is value position → XML
```

### 3.3 The one residual clash, and the rule

The only place both consumers *could* fire is `<` immediately following
a capitalised bare name with no separator: `Foo<div>`. The close-position
generics alternate wins (it is already attached there, and a capitalised
name is overwhelmingly a type reference). To force XML right after a
capitalised name, parenthesise:

```
Foo<Bar>            # generic instantiation (Foo of [Bar])
Foo (<bar/>)        # call Foo with an XML value
```

This costs one pair of parens in a rare position and keeps the rule a
one-liner: **`<` is generics after a capitalised name, XML everywhere
else.** No lookahead beyond the single token already needed to reject a
stray `<`.

### 3.4 How TSX disambiguates — and why boru needs less

TypeScript's JSX dialect faces a *harder* version of this problem
because JS generics can suffix *any* expression and casts use the same
`<T>` shape:

- In `.tsx` files the `<T>expr` **type-assertion/cast is banned
  outright** — JSX wins `<` in expression position, and you must write
  `expr as T` instead.
- Generic **arrow functions** are ambiguous with JSX elements, so TS
  requires a disambiguating hint: `<T,>() => …` (trailing comma) or
  `<T extends unknown>() => …`.
- A JSX element is then recognised by `<` in expression position
  followed by an identifier (or `>` for a fragment).

boru needs none of these hacks. Its generics attach **only to a
capitalised name**, never to an arbitrary expression, so "XML in open
position" and "generics in close position after a Name" are lexically
disjoint. There is no cast form to ban and no arrow-vs-element
ambiguity to annotate. TSX pays for JS's looser grammar; boru's
name-gated generics make the split fall out for free.

A note on tag case: HTML/XML tags are conventionally lowercase
(`<div>`) but React components are capitalised (`<Foo/>`). Because XML
lives in *open* position, a capitalised tag `<Foo/>` is unambiguous too
— there is no preceding value for the generics gate to attach to. Tag
case carries **no** semantic weight in this design (unlike JSX, where
lowercase = host element and capitalised = component); any
component-like dispatch is deferred (§7).


## 4. Interpolation — reuse the template-string machinery

XML literals interpolate boru expressions with `${…}`, identical in
spelling and mechanism to backtick template strings. `XML.0.md §3`
already argued the `${}` choice over bare `{}` (which collides with
map literals and is extremely common in real markup/CSS/code text); we
adopt the same convention so there is one mental model across templates,
program-XML, and data-XML.

### 4.1 The proven single-grammar pattern

Backtick templates already do exactly this without a sub-parser
(`lang/go/CLAUDE.md` "Template string interpolation";
`grammar.go::setupInterpGrammar`; `parse.go::convertInterpGroup`):

- A rule-state flag (`K["boru_tpl"]`) marks "inside template text"; a
  custom `LexMatcher` emits literal-text tokens only while the flag is
  set.
- On `${` the lexer pivots into ordinary expression mode (the matcher
  clears the flag and bumps `dlist`/`dmap` so the inner expression
  parses normally); the matching `}` pops back.
- Nesting works to any depth because each inner expression pushes to
  `valof`, which can open another template (or another XML literal).

XML reuses this verbatim with its own flag, `K["boru_xml"]`: the `xml`
rule sets it while scanning element text, and a text-run matcher emits
literal-text tokens until it sees `<`, `${`, or a closing `>`.

### 4.2 Interpolation sites

```
# text / mixed content
<p>hello ${name}, you have ${count} messages</p>

# child position — a List splices as many children, a scalar/Node as one
<ul>${ items each (x => [<li>${x}</li>]) }</ul>

# attribute value
<a href=${url} class="card">…</a>

# (deferred) tag-name / component position
<${Tag}>…</${Tag}>
```

Splice rule in child position: a `Node/List` contributes each element
as a child (lists of `Node/Xml`/String); a single `Node/Xml`, `String`,
or scalar contributes one child (scalars stringify to a text node).
This mirrors how `word [a b c]` splices a list's elements.

### 4.3 What the literal carries, and when it evaluates

Like `InterpString`, the parsed literal is a **static skeleton plus
embedded expression token-lists**, not an eagerly-built value. At
runtime it evaluates in the current scope — each `${…}` runs as an
ordinary boru expression against the live def stack — and yields a
`Node/Xml`. A literal with no interpolations is a constant `Node/Xml`
(it can be built once at parse/first-eval time). This matches the
`InterpString` "constant-fold when no parts interpolate" behaviour in
`convertInterpGroup`.


## 5. Data structure and API

### 5.1 The types: `Node/Xml` and `Node/FlexXml`

Two types, mirroring the immutable-Node / mutable-Flex relationship of
`Node/Map`↔`Node/Map/FlexMap` and `Node/List`↔`Node/List/FlexList`
(`design/FLEX-NODES.10.md`):

- **`Node/Xml`** — the **immutable** element value. This is the *only*
  XML type the parser ever emits (consistent with FLEX-NODES' invariant
  that "the parser never produces flex nodes"). Words like `each`,
  selectors, and `text` return new `Node/Xml` values; nothing mutates in
  place.
- **`Node/FlexXml`** — the **mutable** build-in-place variant, reached
  via `flex <xml>` and convertible back with `node <flexxml>`, exactly
  as `flex {…}` / `node f` work for maps. This is the tree-builder used
  when assembling XML incrementally (the common "generate a document"
  workload), without leaving the Node world.

Both are **dedicated** kernel-external types (not `Node/Map` subtypes —
see §6 for the trade-off), inhabiting the `Node/` branch and registered
through `RegisterExternalBuiltin` (`eng/go/CLAUDE.md` "Where a Type
Lives"). They take FixedIDs from a **new reserved language band** — the
current map (eng/go/CLAUDE.md "FixedID Allocation") runs through 4999
for natives and 5000-9999 is the catch-all already partly used by module
types (Module 5000, ModuleExport 5001, MiniLangCompiled 5003). Proposal:
carve a documented **XML sub-band 5100-5199**, with `Node/Xml = 5100`
and `Node/FlexXml = 5101`, added to the `fixedid_stability_test.go`
snapshot when implemented.

### 5.2 The payload

```go
// eng/go/payload.go (direct variant: add payloadMarker())
type XmlElementPayload struct {
    Tag  string        // element name, e.g. "div"
    Attr *OrderedMap   // attribute name → String value, insertion-ordered
    Cren []Value       // all children in document order:
                       //   Node/Xml elements and Scalar/String text nodes
}
func (XmlElementPayload) payloadMarker() {}
```

`Node/Xml` holds this by value (immutable, copy-on-return). `Node/FlexXml`
holds a pointer-backed twin (`*FlexXmlData{ … }`) so in-place
append/set are observable through every `Value` copy — the same reason
`FlexList` gets `*FlexListData` rather than reusing `ListPayload`
(FLEX-NODES §Payloads). `flex`/`node` deep-copy across the boundary
(`FlexDeepCopy`/`NodeDeepCopy` already recurse through nested Nodes; the
XML payload joins that recursion).

Text nodes are plain `Scalar/String` values living in `Cren`; there is no
separate text-node type. Comments/PIs, if preserved, are a follow-up
decision (§7).

### 5.3 Well-known keys (stored)

The element exposes exactly three stored keys, terse per boru house style
(cf. the `re` minilang's `{ok ms fst lst n}` and `match`'s `{m i e}` —
short keys are the idiom, not `attributes`/`children`):

| Key | Type | DOM analogue | Meaning |
|-----|------|--------------|---------|
| `tag` | `String` | `localName` | element name |
| `attr` | `Map` | `attributes` | attribute name→value map |
| `cren` | `List` | `childNodes` | **all** children, incl. text |

These are reachable by dotted access (`x.tag`, `x.attr`, `x.cren`) via
the type's `IdealConverter` (§5.4) and `get` behavior. `cren`
("children") deliberately means *all* child nodes including text; the
element-only view is the computed `elem` word below.

(Alternatives considered for the all-children key: `kids`, `nodes`,
`content`. `cren` keeps the four-letter terse shape and pairs visually
with `elem`; it is the maintainer's chosen name.)

### 5.4 Custom Behavior

`Node/Xml` supplies a `TypeBehavior` (`eng/go/CLAUDE.md` "Type
Behavior"):

- **`Format`** serialises back to well-formed XML text (round-trip
  target: `parse xml (format-of x)` ≡ `x` modulo insignificant
  whitespace). This is *why* a dedicated type rather than a Map subtype —
  a Map would render as `{tag:… attr:… cren:…}`, not as `<…>`.
- **`Match`/`Equal`** are structural over `{Tag, Attr, Cren}`.
- **`IdealConverter`** projects to a Map `{tag, attr, cren}` (so
  `convert Map x` works and dotted access has a backing) and could
  project to a List of children for `convert List x`. Every Ideal/Node
  with a converter participates in the `convert` gateway uniformly.

`Node/FlexXml` inherits the `Node/Xml` Behavior through the Parent chain
(Behavior dispatch walks Parent — FLEX-NODES relies on the same for
FlexMap inheriting Map formatting), so it formats and matches as XML
while adding only the mutation surface.

### 5.5 Computed words (not stored)

Derived views are **words**, not stored keys — they are cheap to compute
from `cren` and storing them would duplicate state:

- **`elem`** — element children only (`cren` filtered to `Node/Xml`),
  the DOM `children` view.
- **`text`** — concatenated text content of the whole subtree (DOM
  `textContent`).
- **`xml-attr`** — a node + attribute name → the value or `none`
  (`XML.0.md §4.3`).
- **CSS selectors** — the `cs/…` minilang surface and `xml-is`
  predicate from `XML.0.md §4` are the query companion. `cs/` already
  appears in the minilang catalogue (`MINILANG.5.md §5`, kind `cs`,
  "pairs with XML.0.md"), so selection over a literal-built tree reuses
  that work rather than inventing an XML-only query word.

Construction/mutation on `Node/FlexXml` reuses the flex vocabulary:
`append <child> f`, `set 'class' 'x' f.attr`, etc., per the FLEX-NODES
mutation table.

### 5.6 Aligning `parse xml` to the literal shape

So that a literal and a parsed string are **indistinguishable values**,
the tabnas-backed `parse xml` handler is retrofitted to emit `Node/Xml`
rather than a raw `Node/Map` with `{name, attributes, children}`. The
remap (`parselang.go`, the `"xml"` spec at line 414, and the doc string
at `docs_parselang.go:16`):

```
tabnas  {name, attributes, children}   →   Node/Xml {tag, attr, cren}
```

is a thin adapter over the existing plugin output. After it,
`parse xml '<a/>'` and the literal `<a/>` produce equal values, and
every `Node/Xml` word works on both. (This is an implementation
follow-up, recorded here; this pass changes no Go.)


## 6. Comparison and contrast of approaches

### 6.1 Surface mechanism

| Approach | Structural? | Interpolation | Parser cost | Verdict |
|----------|-------------|---------------|-------------|---------|
| **(a) First-class parser literal** `<tag>…</tag>` | yes — yields `Node/Xml` directly | structural `${}` at text/child/attr | one `val.Open` alternate + a text matcher (reuses angle tokens) | **chosen** — the JSX/TSX analogue, lowest friction at the call site |
| (b) backtick string + `parse xml` | no — parses a string at runtime | only textual `${}` then re-parse; `${expr}` must stringify and survive XML escaping | none (exists today) | works now, but interpolation is textual (injection/escaping hazards) and the result needs an explicit parse step |
| (c) `mini xml '…'` minilang | no — `src` is a string | none structural; vars only via `opts` map (`MINILANG.5.md §3`) | none (rides `mini`) | good for *compiled* DSLs (regex, CSS); wrong shape for tree literals with inline expressions |
| (d) `+xml<…>` mini-literal sugar | no | n/a | new matcher, **but** the `<…>` delimiters clash with both generics and approach (a) | rejected — delimiter collision |

Approaches (b)–(d) all treat XML as *text*; only (a) makes it
*structure* with structural interpolation, which is the whole point of
the JSX analogy. The minilang facility remains the right home for the
*query* side (`cs/` selectors), not the *literal* side.

### 6.2 Value representation: dedicated type vs `Node/Map` subtype

| | dedicated `Node/Xml` (**chosen**) | `Node/Map/Xml` subtype |
|--|-----------------------------------|------------------------|
| map vocabulary (`get`/`each`/…) | via `IdealConverter`/Behavior, opt-in | inherited free (FlexMap precedent) |
| `Format` → `<…>` serialisation | natural (own Behavior) | awkward — base Map formatting fights it |
| identity / `is Xml` dispatch | clean nominal type | conflated with Map slots |
| `flex` → mutable | own `FlexXml` (this design) | would land on generic `FlexMap`, losing the XML tag |
| new machinery | payload + Behavior + 2 FixedIDs | less, but muddier semantics |

The maintainer chose the **dedicated** type: serialise-back fidelity and
a clean `Node/Xml` identity outweigh the free map-vocabulary reuse a
subtype would give. The `IdealConverter`-to-Map keeps map-style access
available without making XML *be* a Map.


## 7. Open questions and phasing

Open questions:

- **Fragments** `<>…</>` — include in v1 or defer? They need a fragment
  sentinel in `Cren` or a list-valued literal result.
- **Tag-name / component interpolation** `<${Tag}>` — JSX's
  host-vs-component split is explicitly *not* adopted (§3.4); a computed
  tag name is a smaller, separable feature.
- **Namespaces / `xmlns`** — store as ordinary attributes, or model
  prefixes structurally? (`XML.0.md §5` uses `xmlns` for module scope —
  a different use of the word.)
- **Whitespace policy** in mixed content — verbatim, collapse, or an
  `xml:space`/`dedent` hint as in `XML.0.md §6.3`.
- **Comments / processing instructions** — preserve in `Cren` (needs a
  representation) or drop.
- **Schema validation** — out of scope here; note as future.
- **Relationship to `XML.0.md`** — an `<boru-embed lang="xml">…</boru-embed>`
  block in a program-XML document should yield the same `Node/Xml` value
  this literal yields; the two designs converge on the type, diverge on
  the surface.

Phasing (each its own implementation task; none in this pass):

1. **Parser hook** — `val.Open` `<` alternate + `xml`/`xelem` rules
   (modelled on `paren`/`pelem` and `angle`/`aelem`) producing an
   `xmlGroup` AST node; conversion to a constant `Node/Xml`.
2. **Type + payload** — register `Node/Xml`/`Node/FlexXml`,
   `XmlElementPayload`/`*FlexXmlData`, Behavior (`Format`, `Match`,
   `Equal`, `IdealConverter`), FixedID snapshot entries.
3. **Interpolation** — `K["boru_xml"]` text matcher + `${}` pivot;
   skeleton-plus-parts evaluation in `stepLiteral`.
4. **`parse xml` alignment** — remap tabnas output to `Node/Xml` (§5.6).
5. **Query/util words** — `elem`, `text`, `xml-attr`, wire `cs/`
   selectors (`MINILANG.5.md`) to `Node/Xml`.

Each phase pairs positive and negative spec rows (the repo's test
discipline): disambiguation (`Box<Int>` vs `<box/>` vs `Foo (<box/>)`),
interpolation splice rules, round-trip `parse xml (format x) ≡ x`, and
`flex`/`node` identity — mirroring `lang/spec/flex.tsv` and
`lang/spec/module-parselang.tsv`.
