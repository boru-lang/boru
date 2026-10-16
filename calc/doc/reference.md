# Reference — the eng API that calc consumes

This is an exhaustive list of every symbol calc imports from
`github.com/boru-lang/boru/core/go` and `github.com/boru-lang/boru/parser/go`,
with a one-line definition each. It exists to be looked up, not
read in order.

The grouping mirrors how the symbols are used in calc's source.
Where two symbols only differ in detail (e.g. `eng.New` vs
`eng.NewTop`), they are listed together.

## Module: `github.com/boru-lang/boru/core/go`

### Registry construction

| Symbol | What it does |
| --- | --- |
| `eng.NewRegistry() (*Registry, error)` | Allocates a fresh registry — empty Defs, empty Types, internal arg/context stacks set up. No words registered. |
| `r.InitRootContext()` | Initialises the root scope used by Store-based `set` / `get` and any context-aware words. Idempotent. |
| `r.MarkReady()` | Marks the registry as ready for use. Calc calls it after `RegisterWords` so subsequent registrations trigger `OnRegisterHook`. |
| `r.SetParseFunc(fn func(string) ([]Value, error))` | Stores a reference to the parser on the registry so word handlers that need to re-parse strings can find it. Calc passes `parser.Parse`. |

### Engine construction and execution

| Symbol | What it does |
| --- | --- |
| `eng.New(r *Registry) *Engine` | Builds a sub-engine. Unhandled FlowCtrl signals at end-of-Run propagate to the parent. |
| `eng.NewTop(r *Registry) *Engine` | Builds a top-level engine. Unhandled FlowCtrl at end-of-Run becomes an error. Calc uses this for every `Eval` call. |
| `e.Run(input []Value) ([]Value, error)` | Runs the input tokens. Returns the residual stack on success, an error on dispatch failure. |

### Native function registration

| Symbol | What it does |
| --- | --- |
| `r.RegisterNativeFunc(NativeFunc)` | Installs a word — name, sigs, handlers. Validates the word name (must be `[a-z][a-z0-9-]*`). |
| `eng.NativeFunc` | Word descriptor — `Name`, `Signatures`. Dispatch policy lives on each sig's `BarrierPos`. |
| `eng.NativeSig` | One overload — `Args`, `Handler`, `Returns`, `BarrierPos`, `QuoteArgs`, `NoEvalArgs`, `FullStack`, `Patterns`, `ReturnsFn`, … |
| `eng.Handler` | `func(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error)` |

`Args` is the signature type tuple. `Returns` declares the
return types for static analysis. `BarrierPos` controls forward
vs stack collection (see `explanation.md`). `FullStack` makes the
handler receive the full carrier stack instead of pre-matched
args.

Calc uses every field except `QuoteArgs`, `NoEvalArgs`,
`Patterns`, and `ReturnsFn` — those are for richer dispatch
patterns the calculator doesn't need.

### Value constructors

| Symbol | What it does |
| --- | --- |
| `eng.NewInteger(n int64) Value` | Build an Integer value. |
| `eng.NewDecimal(f float64) Value` | Build a Decimal value. |

Calc's words only emit Integer and Decimal values. The kernel
exposes constructors for every type — Bool, String, Atom, List,
Map, Word, Path, None, Function, Record, Object — but calc's
vocabulary doesn't need them.

### Value accessors

| Symbol | What it does |
| --- | --- |
| `eng.AsInteger(v Value) (int64, error)` | Extract the `int64` payload, error if `v` isn't an Integer. |
| `eng.AsNumber(v Value) (float64, error)` | Extract as `float64` regardless of whether `v` is Integer or Decimal. |

### Types referenced in signatures

| Symbol | What it does |
| --- | --- |
| `eng.TAny` | Top of the type lattice — matches anything. |
| `eng.TNumber` | `Scalar/Number` — supertype of `TInteger` and `TDecimal`. |
| `eng.TInteger` | `Scalar/Number/Integer`. |
| `eng.TDecimal` | `Scalar/Number/Decimal`. |
| `eng.Type` | The underlying type struct. Calc only ever uses `*eng.Type` pointers handed to it by these package-level vars. |

### Misc

| Symbol | What it does |
| --- | --- |
| `eng.Value` | The kernel's tagged-value type. `Parent *Type` + sealed `Data Payload`. Calc treats it as an opaque token in most places. |
| `eng.Registry` | The dispatch state. Methods used: `RegisterNativeFunc`, `InitRootContext`, `MarkReady`, `SetParseFunc`, `Defs.Names()`. |
| `eng.Engine` | The interpreter loop. Methods used: `Run`. |
| `r.Defs.Names() []string` | All currently-defined word names. The REPL's `:words` meta-command calls this. |

## Module: `github.com/boru-lang/boru/parser/go`

| Symbol | What it does |
| --- | --- |
| `parser.Parse(src string) ([]eng.Value, error)` | Parse boru source into a token slice. Used both at `calc.New` setup (via `r.SetParseFunc`) and per-line by `Calc.Eval`. |

The parser is configured for the canonical boru syntax —
parenthesised forms, dotted access, template strings, typed
lists `[:T]`, typed maps `{:T}`, etc. Calc doesn't use most of
this, but registering the parser keeps the door open for words
that take source strings at runtime.

## What calc does *not* import

Worth listing because the absence is the whole point.

- **`github.com/boru-lang/boru/lang/go`** — the language layer with
  the production word set. Calc deliberately avoids it.
- **`github.com/boru-lang/boru/lang/go/native`** — the engine shim
  that lang re-exports through. Calc reaches the bare eng API.
- **`github.com/boru-lang/boru/lang/go/native`** — array / fetch /
  query natives. None of them belong in a calculator.
- **`modernc.org/sqlite`, `voxgig/struct`, `csv`, `directive`** —
  lang's transitive deps. Calc avoids them by not depending on
  lang, which is what makes its `go.mod` and `go.sum` tiny.

## Cross-references

- For *why* the API looks like this, see
  [explanation.md](explanation.md).
- For *how* to use each piece together, see
  [tutorial.md](tutorial.md).
- For *recipes* keyed by goal, see [how-to.md](how-to.md).
