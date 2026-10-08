# boru How-To Guides

Practical recipes for common tasks. Each guide assumes you know the
basics from the [Tutorial](tutorial.md).


## How to Define and Use Custom Words

Define a reusable word with `def`:

```
def double [dup add]
5 double            => 10
```

Add type checking with `fn`:

```
def square fn [Integer Integer [dup mul]]
square 5            => 25
```

Multiple signatures let one word handle different types:

```
def inc fn [
  Integer Integer [1 add]
  Decimal Decimal [1.0 add]
]
inc 5               => 6
inc 2.5             => 3.5
```


## How to Work with Lists

Create and access:

```
[10, 20, 30]        => [10,20,30]
[10, 20, 30] . 1    => 20
```

Transform with higher-order words:

```
[1, 2, 3] each [dup mul]           => [1,4,9]
[1, 2, 3, 4] where [gt 2]         => [3,4]
[1, 2, 3] fold 0 [add]            => 6
```

Generate sequences:

```
iota 5              => [0,1,2,3,4]
for 5 [dup mul]     => 0 1 4 9 16
```

Reshape and manipulate:

```
iota 6 reshape [2, 3]    => [[0,1,2],[3,4,5]]
[3, 1, 2] grade           => [1,2,0]
[1, 2, 2, 3] unique       => [1,2,3]
```


## How to Work with Maps

Create and access:

```
{name: "Alice", age: 30}
{name: "Alice"} . name     => 'Alice'
```

Evaluate map values with `do`:

```
do {x: [1 add 2], y: [3 mul 4]}    => {x:3,y:12}
```


## How to Use String Interpolation

Use backtick template strings:

```
def name "world"
`hello ${name}`             => 'hello world'
`2 + 3 = ${2 add 3}`       => '2 + 3 = 5'
```

Nest interpolations:

```
`a${`inner ${1 add 2}`}b`  => 'ainner 3b'
```

Escape special characters with `\$` or `` \` ``.


## How to Handle Errors

Catch errors from `do` using the `error` word:

```
do [1 div 0] error [drop 42]       => 42
```

The pattern is: `do [risky-code] error [handler]`. Inside the
handler, the error value is on the stack. Use `drop` to discard it,
or inspect it before recovering.


## How to Run Code in Parallel

Use `await` with a list of code blocks:

```
await [[1 add 2] [3 add 4]]        => [3,7]
```

Each block runs concurrently. Results are collected in order.

### Choose a parallel mode

Pass an options map to select the mode:

```
# Default: all must succeed (like Promise.all)
await (make Options {mode: 'all}) [[sleep 10 1] [sleep 10 2]]

# Full: always returns all results with status (like Promise.allSettled)
await (make Options {mode: 'full}) [[1] [1 div 0]]
=> [{status:'ok,value:1},{status:'error,value:...}]

# First: return the first result (like Promise.race)
await (make Options {mode: 'first}) [[sleep 100 1] [sleep 10 2]]
=> 2

# Any: return the first non-error result (like Promise.any)
await (make Options {mode: 'any}) [[1 div 0] [sleep 10 42]]
=> 42
```


## How to Use Timers

### Sleep

Pause execution for a duration in milliseconds:

```
sleep 100           # pauses 100ms
```

### Schedule deferred work

Use `timeout` for one-shot and `interval` for repeating:

```
def t timeout 1000 [print "done"]
t cancel            # cancel before it fires

def i interval 500 [print "tick"]
i cancel            # stop repeating
```


## How to Read and Write Files

Read a file:

```
read "data.json"                    # auto-detects JSON
read "data.csv" {fmt: 'csv}        # explicit format
```

Write a file:

```
write "out.txt" "hello world"
write "out.json" {x: 1} {fmt: 'json}
```

Supported formats: `json`, `csv`, `tsv`, `jsonic`, `text`.


## How to Use Modules

Define a module:

```
import mymod [
  def helper [dup add]
  def greet fn [String String [`hello ${args.0}`]]
]
```

Import from a file:

```
import "lib/utils.boru"
```

Rename on import:

```
import [helper as h] "lib/utils.boru"
```


## How to Use Variables

Name stack values with `var` for clarity:

```
var [[x] x mul x]
```

With `def` inside `fn`, name parameters:

```
def hyp fn [Number Number Number [
  var [[a b]
    (a mul a) add (b mul b) sqrt
  ]
]]
hyp 3 4             => 5.0
```


## How to Iterate with For Loops

Numeric loop (0 to N-1):

```
for 5 [dup mul]     => 0 1 4 9 16
```

Range loop:

```
for [1, 4] [dup mul]        => 1 4 9
for [0, 10, 2] [dup mul]    => 0 4 16 36 64
```

Use `break` and `continue`:

```
for 10 [dup gt 5 if [break]]
```


## How to Check and Convert Types

Check a value's type:

```
typeof 42           => Integer
typeof "hello"      => String
42 is Number        => true
42 is String        => false
```

Convert between types:

```
convert Integer "42"        => 42
convert String 42           => '42'
convert Decimal 5           => 5.0
```


## Next Steps

- [Tutorial](tutorial.md) — learn boru from first principles
- [Reference](reference.md) — complete word and type reference
- [Explanation](explanation.md) — understand the concatenative model
