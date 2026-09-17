# boru Tutorial

This tutorial walks you through boru from first principles. By the
end, you will be comfortable writing expressions, defining functions,
and working with the type system.


## Getting Started

Build and run the boru REPL:

```bash
cd boru
make build
./boru
```

You will see the `boru>` prompt. Type an expression and press Enter
to evaluate it.


## Your First Expression

boru is a stack machine. Values push onto the stack; words consume
values and push results.

```
boru> 1 2 add
3
```

We pushed `1`, then `2`, then `add` consumed both and pushed `3`.

Try arithmetic:

```
boru> 10 sub 3
7

boru> 4 mul 5
20

boru> 2 pow 10
1024
```

Notice that `sub 3` reads naturally as "subtract 3". The word `sub`
takes the next value (`3`) as its first argument and the top of
stack (`10`) as its second.


## Strings

Strings use double or single quotes. Many words work on strings:

```
boru> "hello" upper
'HELLO'

boru> "hello world" split " "
['hello','world']

boru> "abc" concat "def"
'abcdef'

boru> "hello" contains "ell"
true
```

Template strings use backticks with `${...}` interpolation:

```
boru> def name "world"
boru> `hello ${name}`
'hello world'
```


## The Stack

Values that are not consumed remain on the stack:

```
boru> 1 2 3
1 2 3
```

Stack words let you manipulate values:

```
boru> 5 dup
5 5

boru> 1 2 swap
2 1

boru> 1 2 3 drop
1 2
```


## Lists and Maps

Lists use square brackets:

```
boru> [1, 2, 3]
[1,2,3]
```

Maps use braces with `key:value` pairs:

```
boru> {name: "Alice", age: 30}
{name:'Alice',age:30}
```

Access values with the dot operator:

```
boru> {name: "Alice"} . name
'Alice'

boru> [10, 20, 30] . 1
20
```


## Defining Words

Use `def` to name values or create reusable words:

```
boru> def x 42
boru> x
42

boru> def double [dup add]
boru> 5 double
10

boru> 3 double double
12
```

The list `[dup add]` is a code body. When `double` is called, the
body executes: `dup` copies the top value, then `add` sums both.


## Functions with Types

Use `fn` inside `def` for typed functions:

```
boru> def square fn [Integer Integer [dup mul]]
boru> square 5
25

boru> def greet fn [String String [`hello ${args.0}`]]
boru> greet "world"
'hello world'
```

The three elements define: input type, output type, body.


## Conditionals

The `if` word takes a condition, a then-branch, and an optional
else-branch:

```
boru> 5 gt 3 if ["yes"] ["no"]
'yes'

boru> 0 if ["truthy"] ["falsy"]
'falsy'
```


## Loops

Use `for` for iteration:

```
boru> for 5 [dup mul]
0 1 4 9 16
```

The loop counter starts at 0 and runs to N-1. Each iteration
pushes the counter, then executes the body.

Use a range for more control:

```
boru> for [1, 4] [dup mul]
1 4 9
```


## Evaluation with `do`

The `do` word evaluates a quoted list as a sub-program:

```
boru> do [1 add 2]
3

boru> do {x: [3 add 4], y: 5}
{x:7,y:5}
```


## Error Handling

Errors are values. The `error` word handles them:

```
boru> do [1 div 0] error [drop 42]
42
```

The `do` catches the division-by-zero error and returns it as an
error value. The `error` word then runs the handler list with the
error on the stack. `drop` discards it and `42` is the recovery
value.


## Types

boru has a hierarchical type system. Check types with `typeof`:

```
boru> typeof 42
Integer

boru> typeof "hello"
String

boru> typeof [1, 2]
List
```

Use `is` to test type compatibility:

```
boru> 42 is Integer
true

boru> 42 is Number
true

boru> 42 is String
false
```


## Parallel Execution

Use `await` to run tasks concurrently:

```
boru> await [[1 add 2] [3 add 4]]
[3,7]
```

Each list element runs in its own goroutine. Results are collected
in order.

Use `sleep` to add delays:

```
boru> sleep 100
```

Schedule deferred work with `timeout` and `interval`:

```
boru> def t timeout 1000 [print "done"]
boru> t cancel
```


## Next Steps

- Read the [How-To Guides](how-to.md) for task-oriented recipes
- Consult the [Reference](reference.md) for complete word signatures
- See the [Explanation](explanation.md) for deeper understanding of
  the concatenative model and type system
