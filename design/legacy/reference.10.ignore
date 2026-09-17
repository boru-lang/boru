# boru Reference

Complete reference for boru syntax, types, and words. For learning
boru, see the [Tutorial](tutorial.md). For task-oriented recipes, see
the [How-To Guides](how-to.md).


## Syntax

### Literals

| Syntax | Type | Example |
|--------|------|---------|
| Digits with optional `-` | Integer | `42`, `-5`, `0` |
| Digits with `.` | Decimal | `3.14`, `-0.5` |
| Double or single quotes | String | `"hello"`, `'world'` |
| Backticks with `${...}` | Template string | `` `x = ${x}` `` |
| `true`, `false` | Boolean | `true` |
| Bare unquoted word | Atom (if not a defined word) | `foo` |

### Compound Data

| Syntax | Type | Example |
|--------|------|---------|
| `[a, b, c]` | List | `[1, 2, 3]` |
| `[:type]` | Typed list | `[:String]` |
| `{k: v, ...}` | Map | `{x: 1, y: 2}` |
| `{:type}` | Typed map | `{:Integer}` |

### Comments

| Syntax | Scope |
|--------|-------|
| `# text` | Line comment — to end of line |
| `## text ##` | Block comment — delimited |

### Parentheses

`(expr)` groups a sub-expression, controlling evaluation order.

```
2 mul (3 add 4)     => 14
```

### Template String Escapes

`\\`, `` \` ``, `\$`, `\n`, `\t`, `\r`


## Type System

### Type Hierarchy

```
Any                            -- top of the main hierarchy
None                           -- the unit (its sole inhabitant: `none`)
Never                          -- empty / bottom

Any/Scalar
  Atom
  Boolean                      -- false | true
  Number
    Integer  Decimal
  String
    EmptyString  ProperString
  Path
  Time (external)
    Date  DateTime  Instant  TimeOfDay
    Duration (CalDuration | ClkDuration)
    Timezone

Any/Node
  List (Args, FlexList)
  Map (Inspect, FlexMap)

Any/Ideal
  Class  Resource (Entity)
  Record  Options  Error  Store (System)  Table
  Fetch (Request | Response)                   -- external
  Timeout  Interval                            -- external
  Tensor (Matrix | Vector)                     -- external

Any/Word
  __FW __OP __CP __ED __PE __IS __FN __RC __MK __MV __MD __IN  (internal)

Any/Type
  Function  FunctionSignature
  Disjunct (Enum)
```

Types form a hierarchy with slash-separated paths. A child matches
its parent: `Scalar/String/ProperString` matches `Scalar/String`
matches `Scalar`. A parent does not match a child. `Any` is the
structural root; its name is omitted from rendered paths so
`Scalar.Path()` is just `"Scalar"`.

**Ordering.** Every type has a unified `Rank` integer; `cmp` / `lt` /
`gt` / `sort` run a LCA-Comparer-then-Rank cascade. Type literals
sort before concrete inhabitants of the same family
(`Integer cmp 0 → -1`). Full design in
`lang/doc/design/TYPE-ORDERING.10.md`.

### Short Names

These names expand automatically:

| Short | Full Path |
|-------|-----------|
| `String` | `Scalar/String` |
| `Number` | `Scalar/Number` |
| `Integer` | `Scalar/Number/Integer` |
| `Decimal` | `Scalar/Number/Decimal` |
| `Boolean` | `Scalar/Boolean` |
| `Atom` | `Scalar/Atom` |
| `List` | `Node/List` |
| `Map` | `Node/Map` |
| `Store` | `Ideal/Store` |
| `Table` | `Ideal/Table` |
| `Record` | `Ideal/Record` |
| `Timeout` | `Ideal/Timeout` |
| `Interval` | `Ideal/Interval` |
| `Function` | `Word/Function` |

### Type Operations

| Word | Signature | Description |
|------|-----------|-------------|
| `typeof` | `[Any] -> [Atom]` | Short type name |
| `fulltypeof` | `[Any] -> [Atom]` | Full type path |
| `is` | `[Any, Any] -> [Boolean]` | Type compatibility check |
| `convert` | `[Scalar, Scalar] -> [Scalar]` (`TypeArgs[0]=true`) | Type conversion |
| `base` | `[Any] -> [Any]` | Zero/base value for a type |
| `refine` | `[Any] -> [Type]` / `[Any, Node] -> [Type]` | Build a subtype: `refine <ClassType> {…}` (subclass; base classes use `class`), `refine Record […]`, `refine Table Base` (bind with `def Name (refine …)`) |
| `make` | `[Any, Any] -> [Any]` | Construct typed value |
| `inspect` | `[Any] -> [Map]` | Inspect word or type |


## Execution Model

boru is a stack machine. Tokens are read left to right. Literals push
onto the stack; words consume arguments and push results.

### Argument Collection

Words collect arguments from two sources:

1. **Stack (prefix)**: values already on the stack before the word
2. **Forward (suffix)**: values appearing after the word

Forward arguments fill signature slots first (from `sig[0]`), then
remaining slots are filled from the stack (top-of-stack first). All
positional arrangements are equivalent:

```
1 2 add       => 3    # both from stack
add 1 2       => 3    # both forward
1 add 2       => 3    # one stack, one forward
```

### The `end` Keyword

Stops forward collection, forcing remaining arguments to come from
the stack.

### Evaluation Order

Left-to-right. Each word collects its arguments before the next word
executes. Use parentheses to override.

### Quotation

Lists are evaluated by default. The `quote` word prevents evaluation:

```
quote [1 add 2]     => [1, add, 2]   # unevaluated
```

Code-body positions (in `def`, `fn`, `if`, `for`, `each`, `fold`,
etc.) implicitly suppress evaluation via `NoEvalArgs`.


## Word Reference

### Stack Manipulation

All stack words are stack-only (`/s`).

| Word | Effect | Description |
|------|--------|-------------|
| `dup` | `a -> a a` | Duplicate top |
| `drop` | `a ->` | Remove top |
| `swap` | `a b -> b a` | Exchange top two |
| `over` | `a b -> a b a` | Copy second to top |
| `rot` | `a b c -> b c a` | Rotate top three |
| `nip` | `a b -> b` | Remove second |
| `tuck` | `a b -> b a b` | Copy top below second |
| `dup2` | `a b -> a b a b` | Duplicate top pair |
| `drop2` | `a b ->` | Remove top pair |
| `swap2` | `a b c d -> c d a b` | Swap top two pairs |
| `over2` | `a b c d -> a b c d a b` | Copy third pair to top |
| `pick` | `n -> v` | Copy value at depth n |
| `roll` | `n -> v` | Move value at depth n to top |
| `depth` | `-> n` | Current stack size |
| `stack` | `n -> [...]` | Entire stack as list |

### Arithmetic

All arithmetic words have forward arg collection and work on Integer and
Decimal with automatic promotion.

| Word | Operation | Example |
|------|-----------|---------|
| `add` | `a + b` | `1 add 2 => 3` |
| `sub` | `a - b` | `10 sub 3 => 7` |
| `mul` | `a * b` | `4 mul 5 => 20` |
| `div` | `a / b` | `10 div 2 => 5` |
| `mod` | `a % b` | `10 mod 3 => 1` |
| `pow` | `a ^ b` | `2 pow 10 => 1024` |
| `abs` | `|a|` | `abs -5 => 5` |
| `negate` | `-a` | `negate 5 => -5` |
| `sign` | `-1/0/1` | `sign -5 => -1` |
| `min` | minimum | `3 min 5 => 3` |
| `max` | maximum | `3 max 5 => 5` |

`add` on non-numeric scalars performs string concatenation.

### Rounding

| Word | Description | Example |
|------|-------------|---------|
| `floor` | Round down | `floor 3.7 => 3` |
| `ceil` | Round up | `ceil 3.2 => 4` |
| `round` | Round nearest | `round 3.5 => 4` |
| `trunc` | Truncate toward zero | `trunc 3.9 => 3` |

### Roots, Exponentials, Logarithms

| Word | Description |
|------|-------------|
| `sqrt` | Square root |
| `cbrt` | Cube root |
| `exp` | e^x |
| `log` | Natural logarithm |
| `log2` | Log base 2 |
| `log10` | Log base 10 |

### Trigonometry

| Word | Description |
|------|-------------|
| `sin`, `cos`, `tan` | Standard trig functions |
| `asin`, `acos`, `atan` | Inverse trig functions |
| `atan2` | Two-argument arctangent |
| `hypot` | Hypotenuse |

### Constants

| Word | Value |
|------|-------|
| `math-pi` | 3.141592653589793 |
| `math-e` | 2.718281828459045 |

### String Operations

| Word | Description | Example |
|------|-------------|---------|
| `upper` | Uppercase | `"hello" upper => 'HELLO'` |
| `lower` | Lowercase | `"ABC" lower => 'abc'` |
| `concat` | Join list elements | `["a","b"] concat => 'ab'` |
| `split` | Split string | `"a,b" split ","  => ['a','b']` |
| `contains` | Substring test | `"hello" contains "ell" => true` |
| `indexof` | Find position | `"hello" indexof "ll" => 2` |
| `slice` | Substring | `"hello" slice 1 3 => 'el'` |
| `replace` | Replace pattern | `"hello" replace "l" "r" => 'herro'` |
| `repeat` | Repeat string | `"ab" repeat 3 => 'ababab'` |
| `trim` | Trim whitespace | `"  hi  " trim => 'hi'` |
| `pad` | Pad to width | `"hi" pad 5 => 'hi   '` |
| `match` | Regex match | `"abc" match "b(c)" => {0:'bc',1:'c'}` |
| `escape` | Escape special chars | `"a&b" escape {mode:'html}` |
| `normalize` | Unicode normalization | `normalize {form:'nfc} s` |
| `changecase` | Case conversion | `"fooBar" changecase {style:'snake}` |

### Boolean Operations

| Word | Description | Example |
|------|-------------|---------|
| `and` | Logical AND | `true and false => false` |
| `or` | Logical OR | `true or false => true` |
| `not` | Logical NOT | `not true => false` |
| `xor` | Exclusive OR | `true xor true => false` |
| `nand` | NOT AND | `true nand true => false` |
| `implies` | Implication | `true implies false => false` |

### Comparison

All comparison words route through one total order
(`eng.CompareValues`); cross-family pairs are NOT an error, lists
and maps are ordered (length-first then element-wise / key-wise),
and a bare type literal sorts strictly below every concrete
inhabitant in the same family. See
`lang/doc/design/TYPE-ORDERING.10.md`.

| Word | Description | Example |
|------|-------------|---------|
| `eq` | Equal (cross-leaf magnitude allowed) | `1 eq 1.0 => true` |
| `neq` | Not equal | `1 neq 2 => true` |
| `deq` | Deep / strict-identity equality | `[1,2] deq [1,2] => true` |
| `lt` | Less than | `1 lt 2 => true` · `Integer lt 0 => true` |
| `gt` | Greater than | `2 gt 1 => true` |
| `lte` | Less or equal | `1 lte 1 => true` |
| `gte` | Greater or equal | `2 gte 1 => true` |
| `cmp` | Three-way: `-1`/`0`/`1` | `5 cmp 10 => -1` · `[1 2] cmp [1 3] => -1` |
| `between` | Build closed-interval refinement | `Integer between 10 20` |

### Definition and Scoping

| Word | Description | Example |
|------|-------------|---------|
| `def` | Define a word | `def x 42` |
| `undef` | Remove definition | `undef x` |
| `fn` | Create typed function | `fn [Integer Integer [dup mul]]` |
| `var` | Scoped variable | `var [[x] x mul x]` |
| `args` | Current function args | `args . 0` |
| `quote` | Prevent evaluation | `quote [1 add 2]` |

### Control Flow

| Word | Description | Example |
|------|-------------|---------|
| `if` | Conditional | `5 gt 3 if ["yes"] ["no"] => 'yes'` |
| `for` | Loop | `for 5 [dup mul] => 0 1 4 9 16` |
| `do` | Evaluate list as program | `do [1 add 2] => 3` |
| `error` | Handle error value | `do [1 div 0] error [drop 42]` |
| `break` | Exit loop | `for 10 [dup gt 5 if [break]]` |
| `continue` | Skip to next iteration | `for 10 [dup eq 5 if [continue]]` |

### Array Operations

| Word | Description | Example |
|------|-------------|---------|
| `iota` | Generate 0..N-1 | `iota 5 => [0,1,2,3,4]` |
| `reshape` | Change dimensions | `iota 6 reshape [2,3]` |
| `flatten` | Remove nesting | `[[1,2],[3]] flatten => [1,2,3]` |
| `transpose` | Swap rows/columns | `[[1,2],[3,4]] transpose` |
| `take` | First N elements | `[1,2,3,4] take 2 => [1,2]` |
| `shed` | Remove first N | `[1,2,3,4] shed 2 => [3,4]` |
| `reverse` | Reverse order | `[1,2,3] reverse => [3,2,1]` |
| `unique` | Remove duplicates | `[1,2,2,3] unique => [1,2,3]` |
| `grade` | Sort indices | `[3,1,2] grade => [1,2,0]` |
| `window` | Sliding window | `[1,2,3,4] window 2` |
| `pairs` | Adjacent pairs | `[1,2,3] pairs` |
| `group` | Group by key | `[1,2,3] group [mod 2]` |
| `replicate` | Repeat elements | `[1,2,3] replicate [2,1,3]` |
| `expand` | Expand by mask | `[1,2,3] expand [true,false,true]` |
| `at` | Select by indices | `[10,20,30] at [2,0]` |
| `sortby` | Sort by function | `[3,1,2] sortby []` |
| `member` | Membership test | `[1,2,3] member 2` |

### Higher-Order Array Words

| Word | Description | Example |
|------|-------------|---------|
| `each` | Map function over list | `[1,2,3] each [dup mul]` |
| `fold` | Reduce with accumulator | `[1,2,3] fold 0 [add]` |
| `scan` | Running fold | `[1,2,3] scan 0 [add]` |
| `where` | Filter by predicate | `[1,2,3,4] where [gt 2]` |
| `outer` | Outer product | `[1,2] outer [3,4] [mul]` |
| `inner` | Inner product | `[1,2] inner [3,4] [mul] [add]` |

### Storage

| Word | Description | Example |
|------|-------------|---------|
| `context` | Push context Store | `context` |
| `get` / `.` | Access field/key | `{x:1} . x => 1` |
| `set` | Set Store key | `context set foo 99` |
| `getr` / `!.` | Strict access (error if missing) | `{x:1} !. y => error` |

### I/O

| Word | Description | Example |
|------|-------------|---------|
| `print` | Print with newline | `print "hello"` |
| `printstr` | Print without newline | `printstr "hi"` |
| `read` | Read file | `read "data.json"` |
| `write` | Write file | `write "out.txt" "data"` |
| `stdin` | Stdin path | `read stdin` |
| `stdout` | Stdout path | `write stdout "hi"` |
| `stderr` | Stderr path | `write stderr "err"` |
| `trace` | Traced evaluation | `trace [1 add 2]` |

### Modules

| Word | Description | Example |
|------|-------------|---------|
| `module` | Define module | `module [def x 1]` |
| `import` | Import module/file | `import "lib.boru"` |

### Timer and Concurrency

| Word | Signature | Description |
|------|-----------|-------------|
| `now` | `[] -> [Instant]` | Current UTC instant |
| `sleep` | `[Integer] -> []` | Pause for N milliseconds |
| `timeout` | `[Integer, List] -> [Timeout]` | Schedule callback after N ms |
| `interval` | `[Integer, List] -> [Interval]` | Repeat callback every N ms |
| `cancel` | `[Timeout\|Interval] -> []` | Cancel a timer |
| `await` | `[Options?, List] -> [List\|Any]` | Run blocks in parallel |

**Await modes** (set via Options `{mode: atom}`):

| Mode | Behavior | JS Equivalent |
|------|----------|---------------|
| `'all` (default) | All must succeed; first error fails | `Promise.all` |
| `'full` | Returns all with `{status, value}` | `Promise.allSettled` |
| `'first` | Returns first to complete | `Promise.race` |
| `'any` | Returns first non-error | `Promise.any` |

### Unification

| Word | Description |
|------|-------------|
| `unify` | Unify two values, returns result string and boolean |

### Help

| Word | Description |
|------|-------------|
| `help` | Print help for a topic or general help |


## Argument Ordering

For binary operations, the word reads naturally in infix position.
`10 sub 3` computes `10 - 3 = 7`. Internally, `sub` receives
`args[0]=3` (nearest) and `args[1]=10` (from stack), then computes
`args[1] - args[0]`.

This reversed internal ordering applies to: `sub`, `div`, `mod`,
`pow`, `lt`, `gt`, `lte`, `gte`, `atan2`, `implies`, `contains`,
`indexof`, `split`, `replace`, `pad`.


## Next Steps

- [Tutorial](tutorial.md) — learn boru from first principles
- [How-To Guides](how-to.md) — task-oriented recipes
- [Explanation](explanation.md) — deeper understanding of the model
