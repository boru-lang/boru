# boru `for` Word — Design Review

## Current Specification (SAMPLES.md)

```
for [10] [print i]              — range 0..9, implicit iterator `i`
for [k:1,10] [print k]         — named iterator, start/end
for [0,10,2] [print args.0]    — start/end/step, positional access
```

Implementation via "repeatedly copying body to future stack".
`break`/`continue` via `mark`/`move` (stack jumps).


## Strengths

1. **Two-argument word pattern** — `for [range] [body]` fits the existing
   `[any, any]` signature model, identical to `if [cond] [then]`. The
   parser and engine already handle list arguments.

2. **Body-as-list** — Consistent with `def`, `var`, and `if`. The
   `spliceArg`/`evalCond` patterns in `conditional.go` already demonstrate
   list-body evaluation.

3. **Stack-machine-native** — Repeated token splicing is a clean
   concatenative approach; no new engine loop construct required.


## Weaknesses

### 1. Overloaded range syntax is ambiguous

The three forms use positional semantics inside a single list argument:
- `[10]` — count
- `[k:1,10]` — named var + start + end
- `[0,10,2]` — start, end, step

`[k:1,10]` is problematic: in jsonic, `k:1` inside a list parses as a map
entry, producing `[{k:1}, 10]` — a mixed list of map and integer. This
conflates variable naming with range specification.

### 2. Implicit `i` variable is magic

`for [10] [print i]` introduces an undeclared binding `i`. Unlike `var`,
which makes bindings explicit, magic variables complicate reasoning —
especially with nested loops (does the inner `for` shadow `i`?).

### 3. `args.0` accessor is unprecedented

`args.0` introduces dot-access syntax that exists nowhere else in the
language. The language uses `get` for storage and map fields. This creates
a parser burden for one use case.

### 4. `break`/`continue` via `mark`/`move` is underspecified

`mark`/`move`/`jump` (Stack Jumps section) are themselves unimplemented.
Circular dependency: `for` depends on stack jumps which don't exist.
Stack-position-based control flow is error-prone when the body modifies
the stack.

### 5. Eager body expansion doesn't scale

For `for [1000000] [body]`, eagerly copying the body 1M times to the
future stack is catastrophic. The engine step limit is 1000 (engine.go),
so either large loops are impossible, or the limit must be raised
dramatically, or expansion must be lazy (contradicting the spec).

### 6. No iteration over data structures

The spec covers only numeric ranges. No `for` over lists or maps — the
primary data structures. `map`/`reduce`/`filter` exist as operators, but
a general `for` over collections is absent.


## Recommended Implementation Approaches

### Approach A: Lazy Sub-Engine Execution (Recommended)

Use the sub-engine pattern already proven by `evalCond` in conditional.go:

```go
func registerFor(r *Registry) {
    forHandler := func(args []Value) ([]Value, error) {
        rangeSpec := args[0]
        body := args[1]
        start, end, step := parseRange(rangeSpec)
        iterName := "i" // default

        var results []Value
        for i := start; i < end; i += step {
            installDef(r, iterName, NewInteger(int64(i)))
            sub := New(r)
            out, err := sub.Run(copyList(body))
            uninstallDef(r, iterName)
            if isBreak(err) { break }
            if isContinue(err) { continue }
            if err != nil { return nil, fmt.Errorf("for: %w", err) }
            results = append(results, out...)
        }
        return results, nil
    }
    r.Register("for", Signature{
        Args:    []Type{TList, TList},
        Handler: forHandler,
    })
}
```

Why: no stack expansion, per-iteration step limits, uses existing
`installDef`/`uninstallDef` for the iterator, proven pattern.


### Approach B: Simplify Range Syntax

Replace overloaded forms with explicit syntax:

```
for 10 [print i]                        — integer count from 0
for [1,10] [print i]                    — [start, end]
for [0,10,2] [print i]                  — [start, end, step]
for {var:"k", to:10} [print k]          — map options for named iterator
```

Drop `k:1` naming. Let users name iterators via `var` word:
```
for 10 [var [[k] print k]]
```

This avoids jsonic ambiguity and is consistent with `read`/`write` using
maps for options.


### Approach C: Collection Iteration

Add iteration over lists and maps via a separate `each` word to avoid
ambiguity with numeric ranges:

```
each myList [print i]                   — iterate list elements
each myMap [print key print val]        — iterate map entries
```

Or overload `for` with type-based dispatch:
```
for myList [print i]                    — list iteration (first arg is list, not range-list)
```


### Approach D: Break/Continue via Sentinel Errors

Instead of implementing `mark`/`move`/`jump`, use sentinel errors:

```go
var ErrBreak = fmt.Errorf("break")
var ErrContinue = fmt.Errorf("continue")

r.Register("break", Signature{
    Handler: func(_ []Value) ([]Value, error) { return nil, ErrBreak },
})
r.Register("continue", Signature{
    Handler: func(_ []Value) ([]Value, error) { return nil, ErrContinue },
})
```

The `for` handler catches these; outside `for`, they propagate as errors.
Clean, no new engine machinery, matches standard interpreter patterns.


## Summary

| Aspect | Current Design | Recommendation |
|--------|---------------|----------------|
| Execution model | Eager body copying | Lazy sub-engine per iteration |
| Range syntax | 3 overloaded forms | Integer or list, map for options |
| Iterator naming | Magic `i`, `k:1` | Default `i`, explicit `var` for custom |
| Break/continue | Unimplemented `mark`/`move` | Sentinel errors in for-handler |
| Collection iteration | Not specified | `each` word or type-dispatched `for` |
| Step limit | Engine-global 1000 | Per-iteration limit in sub-engine |

Core takeaway: use the sub-engine pattern already proven by `if` and
`def`. The engine architecture supports this — `for` is `if` in a
Go-level loop, not an eager stack expansion.
