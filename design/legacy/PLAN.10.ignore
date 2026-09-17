# boru Core Execution Loop — Implementation Plan

> **Note:** This plan has been completed. The engine, registry, type
> system, and built-in primitives described below are all implemented.
> A parser (using jsonic, not the stub lexer) was also added, along with
> file I/O, query words, conditionals, and many additional builtins
> beyond the initial scope listed here. See LANGREF.md for the current
> language reference.

## Analysis

boru is a **concatenative stack machine**. The core engine takes the next item,
interprets it as a function, which modifies the stack. Literals self-insert.

This plan focuses **only on the engine**. The input is not source text or
tokens — it is a slice of typed `Value` structs. This lets us build and test
the execution loop directly against internal data structures, without needing
a lexer or parser.

### What exists today

The current scaffolding (`token/`, `lexer/`, `parser/`, `ast/`, `evaluator/`,
`object/`) follows a traditional expression-language pipeline. None of this is
used by the engine plan. The existing packages are **left untouched** — they
will be revisited when a lexer is added later.

### What we are building

A new `internal/engine/` package containing:

| File              | Purpose                                         |
|-------------------|-------------------------------------------------|
| `types.go`        | Hierarchical type system                        |
| `value.go`        | Typed stack values (string, integer, word, forward) |
| `signature.go`    | Function signatures and matching algorithm      |
| `registry.go`     | Function registry and built-in primitives       |
| `engine.go`       | The core execution loop (stack machine)         |
| `engine_test.go`  | Tests against all ENGINE.md examples            |

---

## Steps

### Step 1: Hierarchical Type System (`types.go`)

boru types are **path-like hierarchies**: `string`, `string/proper`,
`number/integer`, `data/map`.

```go
type Type struct {
    Parts []string // e.g. ["string", "proper"]
}

func NewType(path string) Type            // "string/proper" → {["string","proper"]}
func (t Type) Matches(pattern Type) bool  // child matches parent, not vice versa
func (t Type) Specificity() int           // len(Parts)
func (t Type) String() string             // "string/proper"
```

**Matching rules:**
- `string/proper` matches pattern `string` (a child satisfies a parent).
- `string` does NOT match pattern `string/proper` (parent is less specific).
- `any` matches everything.
- Exact match is the most specific.

**Well-known types (constants):**

| Constant       | Path              |
|----------------|-------------------|
| `TAny`         | `any`             |
| `TString`      | `string`          |
| `TStringProper`| `string/proper`   |
| `TStringEmpty` | `string/empty`    |
| `TInteger`     | `number/integer`  |
| `TWord`        | `word`            |
| `TForward`     | `forward`         |

---

### Step 2: Typed Stack Values (`value.go`)

Every entry on the stack is a `Value` with a hierarchical type and a payload.

```go
type Value struct {
    Parent Type
    Data  interface{}
}
```

**Constructors:**

```go
func NewString(s string) Value       // Parent: string/proper (or string/empty if "")
func NewInteger(n int64) Value       // Parent: number/integer
func NewWord(name string) Value      // Parent: word — a function reference
func NewWordModified(name string, argCount int, forceStack, forceForward bool) Value // e.g. lower/1f, lower/s
func NewForward(info ForwardInfo) Value  // Parent: forward
```

**Word payload** (for function references with optional modifiers):

```go
type WordInfo struct {
    Name        string
    ArgCount    int   // -1 = unspecified
    ForceStack bool  // lower/s
    ForceForward bool  // lower/f
}
```

**Forward payload** (for forward argument tracking):

```go
type ForwardInfo struct {
    FuncName     string  // the deferred function
    ExpectedArgs int     // how many forward args needed
    CollectedArgs int    // how many collected so far
}
```

Tests construct values directly:
```go
// Represents the input for "a upper"
input := []Value{NewString("a"), NewWord("upper")}

// Represents the input for "lower B"
input := []Value{NewWord("lower"), NewString("B")}

// Represents the input for "99 lower"
input := []Value{NewInteger(99), NewWord("lower")}
```

---

### Step 3: Function Signatures and Matching (`signature.go`)

A function has a name and one or more **signatures**. Each signature specifies
the types it consumes from prefix (already on the stack) and/or forward (future
values) positions.

```go
type Signature struct {
    Prefix  []Type                              // types on the stack (rightmost = top)
    Suffix  []Type                              // types from future values
    Handler func(args []Value) ([]Value, error) // execution
}
```

**Matching algorithm** — given a function name, the resolved stack, and
optional modifiers (forceStack via /s, forceForward via /f, argCount via /N):

1. Collect all signatures for the function.
2. Filter by modifiers: if `forceStack` (word/s), discard signatures with forward args;
   if `forceForward` (word/f), discard signatures with only prefix args;
   if `argCount >= 0` (word/N), discard signatures where total args != argCount.
3. For each remaining signature, check if the prefix portion matches the top
   of the resolved stack (type matching from Step 1).
4. Suffix signatures are **always candidates** — they don't require the future
   values to exist yet (those will be collected by the forward mechanism).
5. Rank matches by **specificity**: total arg count (longer wins), then sum of
   type specificities (narrower wins). Stack-only wins over forward on ties.
6. Return the best match, or nil if none.

---

### Step 4: Function Registry and Built-ins (`registry.go`)

```go
type Function struct {
    Name       string
    Signatures []Signature
}

type Registry struct {
    funcs map[string]*Function
}

func NewRegistry() *Registry
func (r *Registry) Register(name string, sigs ...Signature)
func (r *Registry) Lookup(name string) *Function
func (r *Registry) Match(name string, stack []Value, modifiers WordInfo) *Signature
```

**Initial built-in functions:**

| Function | Prefix Signature         | Suffix Signature      | Behavior                  |
|----------|--------------------------|-----------------------|---------------------------|
| `upper`  | `[string] → [string]`    | —                     | Uppercase the string      |
| `lower`  | `[string] → [string]`    | `[|string] → [string]`| Lowercase the string      |
| `dup`    | `[any] → [any, any]`     | —                     | Duplicate top of stack    |
| `swap`   | `[any, any] → [any, any]`| —                     | Swap top two values       |
| `drop`   | `[any] → []`             | —                     | Remove top of stack       |

Built-ins are registered in a `DefaultRegistry()` constructor.

---

### Step 5: Core Execution Loop (`engine.go`)

The engine holds a **unified stack** of Values. A **pointer** separates the
resolved past (left) from the unresolved future (right).

```
stack:   [ resolved ... | ^ ... unresolved ]
index:     0  1  ...  p   p+1  ...  n-1
```

```go
type Engine struct {
    stack    []Value
    pointer  int
    registry *Registry
}

func New(registry *Registry) *Engine
func (e *Engine) Run(input []Value) ([]Value, error)
```

**Core loop** (follows ENGINE.md examples step-by-step):

```
Run(input):
    e.stack = copy of input
    e.pointer = 0

    loop:
        if pointer >= len(stack):
            return stack (done)

        val = stack[pointer]

        CASE 1 — Literal (string, integer, etc.):
            // Already resolved. Advance.
            pointer++

        CASE 2 — Word (function reference):
            lookup function in registry

            if not found:
                // Unknown word: treat as a bare string value
                stack[pointer] = NewString(word.Name)
                pointer++
                continue

            match = registry.Match(name, stack[:pointer], word modifiers)

            if match is nil:
                return SignatureError

            if match has only prefix args (or prefix matched):
                // Pop prefix args from stack before pointer
                args = stack[pointer - len(match.Prefix) : pointer]
                results, err = match.Handler(args)
                // Splice: remove args + word, insert results
                stack = stack[:pointer-len(match.Prefix)] + results + stack[pointer+1:]
                // Adjust pointer to re-examine position after results
                pointer = pointer - len(match.Prefix) + len(results)
                continue

            if match has forward args:
                // Insert forward primitive after the word
                fwd = NewForward(ForwardInfo{
                    FuncName:     name,
                    ExpectedArgs: len(match.Suffix),
                })
                // Stack becomes: [..., word, fwd, | ^rest...]
                splice fwd into stack at pointer+1
                pointer += 2  // skip past word and forward
                continue

        CASE 3 — Forward:
            // Should not be at the pointer directly during normal flow.
            // This is handled by Case 4.
            pointer++

        CASE 4 — After resolving/advancing past a literal, check for
                  pending forward:
            When a non-word value at position pointer has been advanced past,
            scan backwards for the nearest Forward value.
            If found:
                // A forward arg has been resolved.
                fwd.CollectedArgs++
                // Move this value to before the function word
                // (the word is just before the forward in the stack)
                move stack[pointer-1] to before the word position
                if fwd.CollectedArgs == fwd.ExpectedArgs:
                    // All args collected. Remove forward, reset pointer
                    // to the word so it retries as a prefix match.
                    remove forward from stack
                    pointer = position of the word
                continue
```

**ENGINE.md `upper` walkthrough** — input: `[NewString("a"), NewWord("upper")]`

```
1. [| ^'a' upper]        pointer=0, val='a'(string)
2. ['a' | ^upper]        pointer=1, val=upper(word), prefix match [string]→[string]
3. args=['a'], handler→'A'
4. ['A' |^]              pointer=1, done
   output: ['A']
```

**ENGINE.md `lower B` walkthrough** — input: `[NewWord("lower"), NewString("B")]`

```
1. [| ^lower 'B']           pointer=0, val=lower(word), no prefix match (stack empty)
                             forward match [|string]→[string]
2. [lower fwd{1,0} | ^'B']  insert forward, pointer=2
3. val='B'(string), advance  pointer=3, but check for pending forward
4. fwd found. CollectedArgs=1 == ExpectedArgs=1.
   Move 'B' before lower:   ['B' lower | ^]
   Remove forward, pointer→lower
5. pointer at lower, prefix match [string]→[string]
   args=['B'], handler→'b'
6. ['b' |^]                  done
   output: ['b']
```

---

### Step 6: Tests (`engine_test.go`)

All tests use typed Values directly — no lexing or parsing.

**Literal self-insert tests:**
```go
{name: "integer literal", input: []Value{NewInteger(42)}, want: []Value{NewInteger(42)}}
{name: "string literal",  input: []Value{NewString("a")}, want: []Value{NewString("a")}}
```

**Prefix function tests:**
```go
{name: "a upper",   input: []Value{NewString("a"), NewWord("upper")},  want: []Value{NewString("A")}}
{name: "C lower",   input: []Value{NewString("C"), NewWord("lower")},  want: []Value{NewString("c")}}
```

**Suffix (forward) function tests:**
```go
{name: "lower B",   input: []Value{NewWord("lower"), NewString("B")},  want: []Value{NewString("b")}}
```

**Signature error tests:**
```go
{name: "99 lower",  input: []Value{NewInteger(99), NewWord("lower")},  wantErr: true}
```

**Modifier tests:**
```go
{name: "lower/1 D", input: []Value{NewWordModified("lower",1,false,true), NewString("D")}, want: []Value{NewString("d")}}
{name: "lower/f E",  input: []Value{NewWordModified("lower",-1,false,true), NewString("E")}, want: []Value{NewString("e")}}
{name: "F lower/s",  input: []Value{NewWordModified("lower",-1,true,false), NewString("F")}, ... } // needs thought on input ordering
```

**Forth primitive tests:**
```go
{name: "dup",  input: []Value{NewInteger(1), NewWord("dup")},  want: []Value{NewInteger(1), NewInteger(1)}}
{name: "swap", input: []Value{NewInteger(1), NewInteger(2), NewWord("swap")}, want: []Value{NewInteger(2), NewInteger(1)}}
{name: "drop", input: []Value{NewInteger(1), NewWord("drop")}, want: []Value{}}
```

---

## Execution Order

```
Step 1: Type system         ─┐
Step 2: Stack values         ├─ Foundation (independent)
                             │
Step 3: Signatures          ─┘
Step 4: Registry + builtins  ← depends on 1, 2, 3
Step 5: Core execution loop  ← depends on 1, 2, 3, 4
Step 6: Tests                ← depends on everything
```

Steps 1-3 have no interdependencies and can be built in parallel.
Step 5 is the critical path.

---

## Scope Boundary

**In scope:** The stack machine engine, typed values, signatures, matching,
forward mechanism, and the initial built-in primitives listed above. All
tested with Go structs directly.

**Out of scope (for now):**
- Lexer / tokenizer / parser — input is typed values, not text
- REPL / CLI wiring — the engine is a library, called from tests
- Jsonic literals (`a:1,b:c`)
- Set predicates (`>2`)
- Storage (`set`/`get`) — can be added later as more builtins
- The existing scaffolding packages — left untouched
