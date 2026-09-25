package help

func init() {
	register(&Entry{
		Word:    "unpack",
		Summary: "Destructure entries of a map into local word bindings.",
		Description: "Extracts entries from a map (or record) and binds each to a bare word " +
			"in the current scope — boru's analogue of JavaScript object destructuring. " +
			"Three selector forms over the same source: `unpack [names] map`, " +
			"`unpack all map`, and `unpack {renames} map`. A fourth form decodes a " +
			"binary frame: `unpack <BinarySpec> b` reads the Bytes `b` against a frame " +
			"type (`def P (refine BinarySpec [layout])`) and returns a Binary instance " +
			"(see `unpack-prefix` for the streaming variant).",
		Notes: []string{
			"`unpack [a b] m` — bind the listed keys: a → m.a, b → m.b.",
			"`unpack all m` — bind every key of the source map.",
			"`unpack {a: x b: y} m` — rename: bind source key a to x, b to y.",
			"Map shorthand works: `unpack {a b} m` ≡ `{a: a b: b}` ≡ `unpack [a b] m`.",
			"A requested/renamed key absent from the source is an error (strict, like getr).",
			"Capitalised (type) names are rejected — unpack binds values only.",
			"Bindings obey scope: torn down at fn-body exit, persist at top level (like def).",
			"`unpack <BinarySpec> b` — decode a binary frame to a Binary instance (a `value:` segment is a match guard, raising no_match on mismatch).",
		},
	})

	register(&Entry{
		Word:    "codequote",
		Summary: "Quote a paren as code (structural quotability).",
		Description: "Like `quote`, but a forward parenthesised group is captured RAW — as " +
			"code (a paren-expression value) — instead of being evaluated. Words become " +
			"atoms and lists are kept unevaluated, exactly like `quote`; only the paren " +
			"handling differs.",
		Notes: []string{
			"`codequote (1 add 2)` — captures the paren as code (NOT the value 3).",
			"`quote (1 add 2)` — by contrast evaluates the paren, then quotes the result (3).",
			"`codequote foo` → Atom; `codequote [1 add 2]` → the list, unevaluated.",
			"For metaprogramming: capture a source expression as data to inspect or splice.",
		},
	})

	register(&Entry{
		Word:    "do",
		Summary: "Evaluate a list or map as code.",
		Description: "Evaluates the elements of a list as boru code. For maps, recursively " +
			"evaluates all values. Used to execute deferred expressions.",
		Notes: []string{
			"Typed lists, typed maps, and record types are not evaluated.",
		},
	})

	register(&Entry{
		Word:    "raise",
		Summary: "Raise an error: construct an Ideal/Error and abort unless caught.",
		Description: "Raises the same Ideal/Error the runtime produces, so one " +
			"`do [body] error [handler]` form catches both native and user errors. " +
			"Three forms: `raise \"msg\"` (code user_error), `raise some_code \"msg\"` " +
			"(a bare word names the code), and `raise {code: c, message: m, …}` — " +
			"extra spec-map keys ride along on the Error value for the handler. " +
			"Handlers read `e.code` (atom), `e.message` (string), and any payload " +
			"keys; `convert Map e` projects the same fields.",
		Examples: []string{
			`raise "boom" ; # [boru/user_error]: boom`,
			`raise bad_input "expected a list" ; # [boru/bad_input]: …`,
			`do [raise {code: nope/q, message: "m", got: 42}] error [var [[e] e.got print]]`,
		},
	})

	register(&Entry{
		Word:    "if",
		Summary: "Conditional execution.",
		Description: "If the condition is true, evaluates the then branch; with a third " +
			"argument, a false condition evaluates the else branch instead. Given a " +
			"single list `[c1 b1 c2 b2 … else]`, the even elements are conditions and " +
			"the following odd element is that clause's body; conditions are tried in " +
			"order and the first true one's body runs (a trailing element is the else).",
		Notes: []string{
			"A condition or branch that is a list is evaluated as code; a plain value is used as-is.",
			"In the clause-list form, conditions after the first match are not evaluated.",
		},
	})

	register(&Entry{
		Word:    "case",
		Summary: "Dispatch on a value: match/block pairs with an optional default.",
		Description: "`case <value> [m1 b1 m2 b2 \u2026 default]` evaluates the value (a " +
			"code-body list executes and its LAST result is captured) and walks the " +
			"clauses in order. A match that is a code-body list executes as if the " +
			"value were already on the stack ([gt 3] runs `v gt 3`) and coerces to " +
			"boolean; any other match UNIFIES with the value (equal scalars/atoms, a " +
			"type literal matches its members). The first matching block runs the " +
			"same way \u2014 value pushed first, like the error handler \u2014 so a block " +
			"list can consume it; a plain-value block is the result as-is. A trailing " +
			"odd element is the default. The stack-value form `v case [clauses]` " +
			"serves error handlers and pipelines. `boru check` requires the clauses " +
			"to be EXHAUSTIVE over the scrutinee's static type: a default-less case " +
			"with a provably-uncoverable value is a check error " +
			"(case_not_exhaustive). The default is not needed when the type " +
			"disjunctions are met — the clauses cover every union alternative, " +
			"every enum member, Boolean's true+false, or the concrete value. " +
			"Comparison predicates prove coverage by interval union ([gt 3] plus " +
			"[lte 3] covers Integer, as does a refine'd range with its " +
			"complement) and [is T] covers nominally; a base-type clause does " +
			"NOT cover a user-defined newtype (nominal boundary). A dynamic " +
			"(untyped or computed) scrutinee REQUIRES a trailing default or an " +
			"Any clause.",
		Examples: []string{
			`case 2 [1 "one" 2 "two" "many"] ; # 'two'`,
			`case 5 [[gt 3] "big" "small"] ; # 'big'`,
			`case "x" [Integer "int" String "str"] ; # 'str' — String provably covers "x", so no default is needed`,
			`case 5 [Integer [mul 10] "other"] ; # 50 — the block sees the value`,
			`do [risky] error [get code case [bad_input/q "rejected" "unexpected"]]`,
			`case 9 [1 "one" 2 "two"] ; # check ERROR case_not_exhaustive — 9 matches no clause; add a default`,
			`def IS (Integer tor String) def f fn x:IS String [case x [Integer "i" String "s"]] ; # exhaustive over IS — no default needed`,
			`def f fn x:IS Integer [case x [Integer 1]] ; # check ERROR case_not_exhaustive — uncovered: String`,
			`def f fn b:Boolean Integer [case b [true 1 false 0]] ; # true+false cover Boolean`,
			`def f fn x:Integer Integer [case x [[gt 3] 1 [lte 3] 2]] ; # exhaustive — the intervals cover Integer`,
			`def Pos refine Integer def f fn x:Pos String [case x [Pos "p"]] ; # a newtype needs its OWN clause — Integer would not cover it`,
			`def f fn x:Any String [case x [1 "one" "other"]] ; # a dynamic scrutinee REQUIRES the default (or an Any clause)`,
		},
	})

	register(&Entry{
		Word:    "while",
		Summary: "Loop while a condition holds.",
		Description: "The traditional condition loop: `while [cond] [body]` " +
			"re-evaluates the condition list before every iteration and runs " +
			"the body while its last value is truthy. Body values accumulate " +
			"onto the stack across iterations, exactly as `for` leaves its " +
			"per-iteration values. break exits with the values collected so " +
			"far; continue abandons the current body round. Every region is " +
			"engine-stepped, so the step budget bounds the loop — a " +
			"non-terminating condition trips evaluation_limit rather than " +
			"hanging.",
		Examples: []string{
			`while [false] ['x'] 'done' ; # => done — a falsy condition runs the body zero times`,
			`while [true] [break] 'ended' ; # => ended — break exits with the values collected so far`,
		},
		Notes: []string{
			"The condition is a truthiness read, not Boolean-only.",
			"An empty condition region is a loud runtime_error.",
			"Compilation is refused today — the loop runs on the interpreter " +
				"(lang/spec/frontier/frontier-while.tsv pins the ledger).",
		},
	})

	register(&Entry{
		Word:    "for",
		Summary: "Iterate over a numeric range.",
		Description: "Iterates over a range and evaluates the body list for each step. " +
			"With an integer n, iterates from 0 to n-1. With a list, specifies " +
			"[end], [start end], or [start end step]. The loop variable i holds " +
			"the current index.",
		Notes: []string{
			"The loop variable is named i by default.",
			"Use break to exit early; use continue to skip to the next iteration.",
		},
	})

	register(&Entry{
		Word:        "break",
		Summary:     "Exit the current for loop early.",
		Description: "Immediately terminates the innermost for loop. Stack-only.",
	})

	register(&Entry{
		Word:        "continue",
		Summary:     "Skip to the next iteration of the current for loop.",
		Description: "Skips the rest of the current loop body and continues with the next iteration. Stack-only.",
	})

	register(&Entry{
		Word:    "def",
		Summary: "Define a new word.",
		Description: "Defines a new word with the given name and body. When the word is later " +
			"invoked, the body is evaluated. Definitions are stackable: multiple defs " +
			"for the same name stack, and undef pops the top definition. " +
			"Has forward arg collection, so both 'def name body' and 'body def name' work " +
			"via flexible argument matching.",
		Notes: []string{
			"def accepts a word or string as the name.",
			"Use fn with def to define typed functions with parameters.",
			"Use undef to remove the most recent definition.",
			"The fn form `def name fn [...]` is its own signature: the " +
				"literal word fn is a keyword slot, so the function is " +
				"constructed structurally at dispatch.",
		},
		Examples: []string{
			`def x 5 x mul 2 ; # 10`,
			`def xs [1 add 2] xs ; # [3]`,
			`def double fn n:Integer Integer [n mul 2] double 4 ; # 8`,
		},
	})

	register(&Entry{
		Word:    "undef",
		Summary: "Remove the most recent definition of a word.",
		Description: "Pops the most recent definition of the named word, restoring any previous " +
			"definition if one exists.",
	})

	register(&Entry{
		Word:    "var",
		Summary: "Define scoped variables with automatic cleanup.",
		Description: "Takes a list whose first element is a list of variable declarations " +
			"and whose remaining elements are the body. Each declaration is either a bare " +
			"word (takes value from stack) or a [name value] list. Variables are automatically " +
			"undefined after the body executes.",
		Notes: []string{
			"Variables are scoped: they are undefined when the body finishes.",
			"Bare word declarations consume values from the stack.",
		},
	})

	register(&Entry{
		Word:    "fn",
		Summary: "Create a function value with typed parameters.",
		Description: "Parses signature triples [input-types output-types body] " +
			"into a function value. Two forms: the spec-list form `fn [[input] [output] [body] …]` " +
			"takes a single list of triples (overloads are additional triples in the same list); " +
			"the 3-arg form `fn input output body` takes one triple directly, for the common " +
			"single-signature case. The 3-arg form requires a NON-LIST input (a bare type, a " +
			"named param like x:Integer, or a literal pattern) — a list input always selects " +
			"the spec-list form, so multi-param triples need the list form. Usually used with " +
			"def to bind the function to a name. Parameters can be named (x:Integer) or unnamed, " +
			"and so can returns — a name in the output slot is documentation, since returns are " +
			"positional and bind nothing.",
		Notes: []string{
			"Spec-list form: fn takes a single list argument whose length is divisible by 3.",
			"Each triple is: [input-params] [output-types] [body].",
			"3-arg form: fn input output body — one triple; the input must be non-list and the body must be a […] list ([(tnot List) Any List] in the signature). fn x:Integer [Integer] [x mul 2] ≡ fn [[x:Integer] [Integer] [x mul 2]].",
			"Named params use pair syntax: name:Type. Unnamed params are bare types. (An explicit map like {x: Integer} declares a single Map-typed param, not a named binding.)",
			"The output slot takes the same two spellings, so fn [x:A y:B [body]] is fn [[x:A] [y:B] [body]].",
			"Literal values (like 0) can be used as type constraints for pattern matching — in the OUTPUT slot too, where a literal declares a return that must equal it: fn x:String 22 [22] is well-typed, fn x:String 22 [33] is not.",
			"Use with def to bind: def name fn [...] or fn [...] def name.",
		},
		Examples: []string{
			`def double fn x:Integer Integer [x mul 2] double 5 ; # => 10`,
			`def triple fn x:Integer Integer [x mul 3] triple 5 ; # => 15 (3-arg form)`,
			`def add10 fn Integer Integer [10 add] add10 5 ; # => 15 (unnamed param)`,
			`def si1 fn [s:String i:Integer [convert Integer s]] si1 '1' ; # => 1 (named return)`,
		},
	})

	register(&Entry{
		Word:    "args",
		Summary: "Push the current function's argument list.",
		Description: "Returns the list of arguments passed to the current fn-defined function. " +
			"Stack-only.",
	})
}

func init() {
	register(&Entry{
		Word:        "afn",
		Summary:     "Build an anonymous function (lambda). `=>` is sugar for afn.",
		Description: "`input afn body` — written input => body — makes a one-signature anonymous fn. Params follow fn's abbreviations: ([x:Integer] => [mul x 2]). Used wherever a fn value is expected.",
	})
	register(&Entry{
		Word:        "guard",
		Summary:     "Pass a value through only when a condition holds: `cond guard value`.",
		Description: "Returns the value when the condition is true, and none when it is false — a concise conditional gate.",
		Examples:    []string{`true guard 42 ; # => 42`, `false guard 42 ; # => None`},
	})
	register(&Entry{
		Word:        "error",
		Summary:     "Recover from an error: `value error [handler]`.",
		Description: "If the value is an Error, the handler list runs (with the error on the stack) to produce a fallback; otherwise the value passes through unchanged. boru's try/catch combinator.",
	})
	register(&Entry{
		Word:        "force-arity",
		Summary:     "Wrap a function to collect exactly n forward arguments (the /N suffix).",
		Description: "`force-arity n fn` fixes how many arguments the fn forward-collects, overriding its declared arity.",
		Examples:    []string{`3 (force-arity 1 add) 4 ; # => 7`},
	})
	register(&Entry{
		Word:        "usurp",
		Summary:     "Wrap a function so it forward-collects its arguments (the /u suffix).",
		Description: "`usurp fn` returns a new fn that reads its args from the tokens ahead — handy for invoking a referenced fn in forward form.",
		Examples:    []string{`def f fn x:Integer Integer [mul x 2] 3 (usurp f) ; # => 6`},
	})
	register(&Entry{
		Word:        "forward-args",
		Summary:     "Wrap a function to force forward argument collection (the /f suffix).",
		Description: "Forces the fn to take its arguments from the following tokens regardless of its declared barrier.",
	})
	register(&Entry{
		Word:        "stack-args",
		Summary:     "Wrap a function to force stack argument collection (the /s suffix).",
		Description: "Forces the fn to take its arguments from the stack only, never from forward tokens.",
	})
}
