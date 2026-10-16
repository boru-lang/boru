# Object Methods and Self/This Binding

> **Status: descriptive + exploratory (design space).** No implementation
> work proposed.
>
> **Syntax note (2026-07):** this note predates the Object/Array removal.
> The open `Object` container and the `refine Object {…}` form shown
> throughout are **gone** — user records are now defined with the `class`
> word (`def Person class {name:String age:Integer}`; subclass with
> `refine <ClassType> {…}`). The discussion of methods as free functions
> over an instance still applies to class instances; read every
> `refine Object {…}` below as `class {…}`.

## Question

When a Function value is stored as a field on a boru Object, and the
user invokes it via `obj.method args`, does the method body have any
implicit access to the owning object — the equivalent of `this` or
`self` in object-oriented languages?

**Answer: no.** boru has no implicit receiver binding. This document
records why, what idioms cover the gap, and what changes would be
needed to add `this`/`self` if that became a design goal.

## Current state — empirically verified

The body of a Function stored on an object cannot see sibling fields
by name, and there is no `this` / `self` binding:

```boru
def Box (refine Object {value:Integer add-to:Function})
def b (make Box {value:10 add-to:([n:Integer] => [n value add])})
5 b.add-to
→ ERROR: undefined word: value
```

```boru
add-to:([n:Integer] => [n this.value add])
5 b.add-to
→ ERROR: undefined word: this
```

What DOES work is normal lexical capture — outer-scope bindings the
lambda saw at construction time:

```boru
def some-outer 100
def Box (refine Object {use-outer:Function})
def b (make Box {use-outer:([n:Integer] => [n some-outer add])})
5 b.use-outer
→ [105]
```

## Architectural reason

`refine Object {…}` produces a typed-record with prototype-based
**field inheritance** (`ObjectInstanceInfo` in `core/go/value.go:373`
holds `Fields *OrderedMap` and `Prototype *ObjectInstanceInfo`). The
prototype chain is consulted by `GetField` (line 381) for field
*lookup* only; it is not a method-dispatch table.

A Function stored as a field is just a value. The two-step `b.method`
parses as `b get method`:

1. `get` retrieves the field value from `b`.
2. The Function lands on the stack with whatever lexical scope it
   captured at `make`-time.
3. If sufficient arguments are staged (forward or stack), it
   auto-dispatches through `execFnDefLiteral`
   (`core/go/engine.go:1506`) via the normal sig-matching path.

There is no separate "method call" pathway in the engine. There is no
"set receiver" step. `b` was consumed by `get` to extract the field;
by the time the Function dispatches, the engine has no reference to
the owning object at all.

This matches boru's concatenative model: values flow through a
unified stack, and dispatch is by sig, not by receiver type. The
"object" abstraction is a typed-record-with-inheritance, not a
collection of methods bound to a receiver.

## Three idioms that cover the gap today

### Idiom 1 — Free functions taking the receiver explicitly

The most idiomatic boru pattern. A "method" is just a normal `fn`
that takes the object as one of its arguments:

```boru
def Counter (refine Object {n:Integer})
def bump fn [[c:Counter] [Counter] [
  make Counter {n: (c.n 1 add)}
]]

def c0 (make Counter {n:5})
c0 bump            # → Counter{n:6}
c0 bump bump       # → Counter{n:7}
```

Dispatch goes through normal sig matching, so you can overload `bump`
for different receiver types. Go, Rust, and Haskell take this
approach. The dot is purely for field access; the "receiver" is just
another argument.

**Pros**: no new language machinery; integrates with type-driven
dispatch; methods are first-class words.
**Cons**: no namespacing — `bump` is global rather than scoped to
`Counter`. Conflicts with other types' `bump` methods unless renamed.

### Idiom 2 — Constructor closures

Wrap construction in a `fn` so the body's lexical scope captures the
intended state:

```boru
def make-counter fn [[init:Integer] [Object] [
  make Counter {
    n: init
    bump-fn: ([] => [init 1 add])    # closure captures init
  }
]]

def c (5 make-counter)
c.bump-fn          # 0-arg Function — stays as data, see notes below
```

Per `lang/go/CLAUDE.md` "Closures and Capture": `init` is snapshotted
into `bump-fn`'s `FnDefInfo.Captured` at construction. The closure
sees `init` regardless of what happens to `make-counter`'s scope
after it returns.

**Pros**: works with existing closure semantics; no kernel changes.
**Cons**: capture is a **snapshot at construction**, not a live
reference. If `c.n` is later mutated, `bump-fn` still sees the
original `init`. To track field changes the lambda must re-read
the field via dot-access on a captured object reference (which
introduces a self-reference cycle at construction — see Idiom 3).

### Idiom 3 — Map-as-object (the `rand.with-seed` pattern)

A constructor returns an `OrderedMap` of FnDef wrappers, each closing
over shared state. Each "method" is a real FnDef in the map:

```boru
def r (rand.with-seed 42)
r.int 0 100        # dispatches r.int against the captured rng state
```

Under the hood (`lang/go/modules/rand.go:buildRandExportsForState`),
each method's handler closes over a private `*randState`. The "this"
equivalent is the captured state in the Go closure — invisible from
boru but providing the same behavioural effect (per-instance state,
shared across methods on the same instance).

**Pros**: each call dispatches with live state access; methods are
real callable values; no global state.
**Cons**: requires the methods to be **built in Go**, not authored
in boru source. There is no boru-source-level way to write this
pattern today because:

- boru closures capture by snapshot, not by reference.
- boru has no mutable cell or "ref" abstraction that the user can
  thread through closure construction.
- The OrderedMap holding the methods doesn't exist yet at the time
  each method body is constructed, so a "this" reference at boru
  source level can't cyclically refer to the enclosing map.

This is the closest boru gets to "methods with implicit this" — but
it lives in the Go layer.

## Design space for adding implicit `self`/`this`

Three plausible designs, in order of intrusiveness:

### Option A — Implicit `self` parameter at dispatch time

When `obj.method args` dispatches a Function-typed field of `obj`,
prepend `obj` as an extra arg to the call:

```boru
def Box (refine Object {value:Integer get-val:Function})
def b (make Box {value:10 get-val:([self:Box] => [self.value])})
b.get-val          # → 10
```

The `get-val` lambda declares `self:Box` as its first param; the
engine fills it from the object that owned the dot-access. The dot
itself becomes a "method-call" form when the field's value is a
Function, distinct from the "field-fetch" form for non-Function
values.

**Engine change**: `execFnDefLiteral` (or a new path it routes to)
needs to detect "this dispatch was reached via dot-access on an
object" and inject the receiver. Today `get` retrieves the value and
the engine forgets the container; this would have to be reworked so
the container survives into the dispatch.

**Surface cost**: the dot form becomes type-dependent. `b.x` returns
the field value if `x` is non-Function and dispatches a method if `x`
is Function. This dual semantic is what Python and JavaScript do —
familiar but surprising in a concatenative language where dot
historically means just "get key".

**Compatibility**: existing code that retrieves a Function field as
data (rare but legal) would break. Could be opt-in via a new sig
marker on the field's type (`get-val:MethodFunction` vs `:Function`).

### Option B — `make`-time receiver injection into Function fields

When `make Foo {…}` sees a Function-typed field, rewrite the
Function's `FnDefInfo` to install the new instance as a captured
binding under a documented name (`self` or `this`):

```boru
def Box (refine Object {value:Integer get-val:Function})
def b (make Box {value:10 get-val:([] => [self.value])})
b.get-val do       # `self` was injected into the lambda at make-time
```

The lambda body authors reference `self` as a normal captured
binding. `make` walks the data, finds Function fields, and modifies
each one's `Captured` list to include `{Name: "self", Value: <new
instance>}`. Same with prototype chain: `super` could be injected
similarly for the parent instance.

**Engine change**: `make`'s field-resolution path needs a hook to
mutate Function values during construction. The `FnDefInfo.Captured`
field already exists — extend it with the receiver and use the
standard closure-install machinery.

**Surface cost**: `self` becomes a reserved word inside Function
fields of objects. Existing user code that uses `self` as a binding
elsewhere is unaffected (the new binding is only installed for
fields of Object types, not for free-standing lambdas).

**Wrinkle**: when `b` is later passed somewhere else and its method
is extracted (`def f b.get-val; f do`), the captured `self` still
points to `b`. That matches JavaScript's bound-method behaviour but
might surprise users who expect "extracted method loses its
receiver". Document the semantic.

**Wrinkle 2**: cyclic data — `b` captures `self=b`. The cycle is fine
for GC but inspect/print/serialize need to detect it.

### Option C — Receiver explicit in sig

Don't change dispatch; instead make the `self`-pattern explicit by
documenting that methods declare `self` as their first param and the
user passes the receiver:

```boru
def Box (refine Object {value:Integer get-val:Function})
def b (make Box {value:10 get-val:([self:Box] => [self.value])})
b b.get-val        # explicit pass-self
```

This is essentially Idiom 1 but with the method stored on the object
for namespacing. No kernel changes; pure convention.

**Pros**: zero language complexity; reuses existing dispatch
verbatim.
**Cons**: the `b b.method` repetition is awkward. The user could
introduce a helper word (`obj method-name dispatch-self`) but that's
just sugar.

## Recommendation

**Do not implement implicit `self`/`this` at the kernel layer
unless a concrete use case demands it.** The current free-function-
plus-receiver pattern (Idiom 1) covers nearly every real OO use
case, integrates cleanly with boru's sig-driven dispatch, and keeps
the language model simple. The Map-as-object pattern (Idiom 3)
covers the per-instance-state case for modules.

If "object-style methods" turn out to be a frequent user pain point
— evidenced by the same workaround appearing across multiple
modules or by user feedback specifically asking for them — revisit
Option B (`make`-time receiver injection). It's the cleanest of
the three because:

- It piggybacks on existing closure machinery (`FnDefInfo.Captured`).
- The dispatch path is unchanged (`execFnDefLiteral` already handles
  Function values with captures).
- The change is localised to `make`'s field-walking step.

Avoid Option A — overloading the dot operator with method-dispatch
semantics breaks the "dot is just get" mental model that the parser
codifies (`parser/go/parse.go:173`).

## Open questions

- Does the prototype chain need its own injection? `super.method`
  inside a method body, for parent-type access?
- Does serialisation (`format/serialize`) need to skip Function
  fields to avoid the cyclic-capture problem? Today serialising an
  object with a Function field probably fails or produces opaque
  output anyway.
- Should `inspect obj` show captured `self` bindings on method
  fields? Probably no — they'd dominate the output and create
  recursive-display issues.

These belong in a Phase-2 proposal if/when implicit-self is
actually picked up.

## References

- `core/go/value.go:373` — `ObjectInstanceInfo`.
- `core/go/value.go:381` — `GetField` (prototype-chain lookup).
- `core/go/core_make.go:213` — `MakeObject`.
- `core/go/engine.go:1506` — `execFnDefLiteral` (Function-on-stack
  dispatch).
- `lang/go/CLAUDE.md` "Closures and Capture" — existing lexical
  capture semantics.
- `lang/go/modules/rand.go::buildRandExportsForState` — current
  Map-as-object pattern in production.
