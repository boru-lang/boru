package help

func init() {
	register(&Entry{
		Word:    "convert",
		Summary: "Convert a value to a different type.",
		Description: "Converts the first argument to the target type. Supports integer, float, " +
			"string, boolean conversions. An optional options Map provides settings: " +
			"`base` (radix for numeric conversions, e.g. {base:'hex'}), `truthy` " +
			"(when true, `convert Boolean` parses a String YAML-style — " +
			"y/yes/true/on and n/no/false/off, case-insensitive — falling back to presence " +
			"coercion for any other string), and `accuracy` (with the companion " +
			"`places`) which opts in to the otherwise-refused `Float → BigDecimal` " +
			"conversion. A binary Float is inexact, so `accuracy` states which reading " +
			"you want: 'shortest' (the shortest decimal that round-trips — what the " +
			"literal reads as), 'exact' (the true dyadic value the float64 holds), or " +
			"'round' with `places:N` (the exact value rounded to N decimal places, " +
			"half away from zero). It is inert for any non-Float / non-BigDecimal " +
			"conversion. Boolean conversion is otherwise pure " +
			"presence coercion: empty String / 0 / none / empty collection are false, " +
			"everything else is true.",
		Examples: []string{
			`convert Integer "42" ; # 42`,
			`convert Integer {base:'hex'} "ff" ; # 255`,
			`convert Boolean {truthy:true} "no" ; # false (YAML token)`,
			`convert Boolean {truthy:true} "meh" ; # true  (not a token, non-empty)`,
			`convert BigDecimal {accuracy:'shortest'} 3.14 ; # 0d3.14`,
			`convert BigDecimal {accuracy:'round' places:2} 3.14159 ; # 0d3.14`,
		},
	})

	register(&Entry{
		Word:        "typeof",
		Summary:     "Return the type name of a value.",
		Description: "Consumes the top value and pushes its type name as an atom.",
	})

	register(&Entry{
		Word:    "inspect",
		Summary: "Return a detailed map describing a word, value, or type.",
		Description: "For a word: a map with name, type, kind, and signatures — each " +
			"signature carrying both its args and its returns, so the map is the " +
			"whole contract rather than half of it. A word bound to a function " +
			"reports type 'Function' (ADR-011); a word bound to a plain value " +
			"reports that value's type leaf and the value itself. For a value or " +
			"type: a map with type ('Type' for a type value, otherwise the value's " +
			"type leaf), struct (the underlying-structure leaf, for types), kind, and " +
			"kind-specific fields (fields / alternatives / child / signatures / …).",
	})

	register(&Entry{
		Word:    "make",
		Summary: "Create a value conforming to a type.",
		Description: "Constructs a value of the given type from the provided data. " +
			"For tables, creates table rows from list data. The Scalar/Micron " +
			"structured scalars (Pathon / Emailon / Urlon / Ipon / Hoston / Semveron / " +
			"Cidron / Macon / Coloron / Mimon / Qion / Phonon and user Microns) " +
			"construct from a validated string OR a map of named " +
			"fields — `make Emailon 'a@b.co'` ≡ `make Emailon {user:'a' host:'b.co'}`, " +
			"`make Ipon '2001:db8::1'`, `make Hoston 'example.com:8080'` (host:port, " +
			"optional port), `make Semveron '1.2.3-rc.1'` (SemVer 2.0.0, ordered by " +
			"precedence), `make Cidron '10.0.0.0/8'` (CIDR block), " +
			"`make Macon '00:1a:2b:3c:4d:5e'` (MAC address), " +
			"`make Coloron '#ff0000'` (sRGB colour), `make Mimon 'text/html'` " +
			"(media type), `make Qion 'USD 12.50'` (money — exact Decimal + ISO 4217), " +
			"`make Phonon '+14155552671'` (E.164 phone number) — and expose " +
			"read-only properties via dot " +
			"(`e.host`, `u.port`, `i.addr`, `h.authority`, `s.major`, `s.stable`).",
		Examples: []string{
			`make Point {} ; # fresh instance, all fields at their defaults`,
			`make Point {x:3 y:4} ; # instance with field overrides`,
			`(make Counter {}).count ; # parenthesise to read a field off a fresh make`,
			`make Emailon 'alice@example.com' ; # validated structured scalar (Micron)`,
			`(make Urlon 'https://x.com/a').host ; # 'x.com' — Microns have properties`,
		},
	})

	register(&Entry{
		Word:    "refine",
		Summary: "Construct a (sub)type from a base type.",
		Description: "Builds a (sub)type: `refine ParentClass {fields}` a subclass " +
			"(classes are defined with `class {fields}`), `refine Record [pairs]` " +
			"a record type, `refine Table recordtype` a table type, `refine List` " +
			"a bare nominal subtype. Pair with `def Name (refine …)` to bind it. " +
			"`def Baron refine Micron {foo:String}` mints a user structured scalar " +
			"(content-equal, immutable, property-readable); every name bound under " +
			"Scalar/Micron must end in the suffix 'on'. A Micron LEAF (Emailon, " +
			"Urlon, Pathon, Ipon, Hoston, Semveron, Cidron, Macon, Coloron, Mimon, " +
			"Qion, Phonon) only refines nominally " +
			"(`def Workon refine Emailon`) — its field set is its validation contract.",
	})

	register(&Entry{
		Word:    "class",
		Summary: "Define a class: a sealed nominal record type.",
		Description: "`def Foo class {schema}` mints a class type under Ideal/Class. " +
			"Schema entries: a TYPE value ({name:String}) declares a REQUIRED field; a " +
			"CONCRETE value ({retries:3}) declares a default, typed by the value itself. " +
			"Instances (`make Foo {\u2026}`) are flat \u2014 every field, own and inherited, " +
			"resolves at make \u2014 and sealed: writing an undeclared field raises " +
			"sealed_field. Subclass with refine: `def Bar refine Foo {\u2026}`. For open, " +
			"unsealed mutable data use a FlexMap (`flex {\u2026}`) instead.",
		Examples: []string{
			`def Point class {x:1 y:2} ; # x,y default to 1,2`,
			`def p (make Point {x:9}) ; # p.x => 9, p.y => 2`,
			`def Point3 refine Point {z:3} ; # subclass, inherits x,y`,
			`p set w 5 ; # ERROR sealed_field`,
		},
	})

	register(&Entry{
		Word:    "surface",
		Summary: "Declare a surface: a pure operation contract types can expose.",
		Description: "`def Shape surface {schema}` mints a surface type under " +
			"Ideal/Surface: a named set of required operation shapes with no bodies " +
			"and no state. Each schema entry maps an operation name to an fnsig " +
			"shape, with `Self` marking the positions the conforming type occupies. " +
			"Conformance is EXPLICIT \u2014 declare it with `<Type> exposes Shape` " +
			"(no structural duck typing). Members then dispatch through surface-typed " +
			"fn params, answer `is`, and ride the tor/tand/tnot type algebra.",
		Examples: []string{
			`def Shape surface {area:(fnsig [[Self] [Float]])}`,
			`def Circle class {r:1.0}`,
			`def area fn c:Circle Float [(c get r) mul 6.28]`,
			`Circle exposes Shape ; # loud completeness check`,
			`def total fn s:Shape Float [area s]`,
		},
	})

	register(&Entry{
		Word:    "exposes",
		Summary: "Declare (and loudly check) that a type provides a surface.",
		Description: "`<Type> exposes <Surface>` checks the overload table for every " +
			"operation the surface requires, with `Self` replaced by the type " +
			"(contravariant params, covariant returns). Any gap raises " +
			"surface_unsatisfied listing every missing operation with its expected " +
			"signature; success registers the type in the surface's conformance set. " +
			"Idempotent. Subclass instances of an exposer conform automatically. The " +
			"check runs at declaration time \u2014 a later undef of an operation is " +
			"not re-checked (the call then fails loudly downstream).",
		Examples: []string{
			`def Shape surface {area:(fnsig [[Self] [Float]])}`,
			`def Circle class {r:1.0}`,
			`def area fn c:Circle Float [3.14]`,
			`Circle exposes Shape`,
			`(make Circle {}) is Shape ; # true`,
		},
	})

	register(&Entry{
		Word:    "gen",
		Summary: "Declare type parameters for a generic schema.",
		Description: "`def Box gen [T] refine Record [value:T]` declares a generic " +
			"SCHEMA: gen's list names the type parameters (bare name = " +
			"unconstrained; `(T extends C)` bounds it; `(E default D)` defaults " +
			"it; both combine). The placeholders are bound while the following " +
			"type constructor (refine Record / fnsig) builds its body, so `T` " +
			"resolves in field and signature positions. Instantiate with " +
			"`Box of [Integer]`. Recursion uses `Self of [T]` (the schema's own " +
			"name is unbound while its body builds).",
		Examples: []string{
			`def Box gen [T] refine Record [value:T]`,
			`def Pair gen [K V] refine Record [key:K value:V]`,
			`def Sorted gen [(T extends Number)] refine Record [items:[:T]]`,
			`def Result gen [T (E default Error)] refine Record [ok:T err:E]`,
			`Box of [Integer] ; # record{value:Integer}`,
		},
	})

	register(&Entry{
		Word:    "of",
		Summary: "Instantiate a generic schema with type arguments.",
		Description: "`Box of [Integer]` substitutes the schema's parameters: arity " +
			"is checked (defaults fill omitted trailing parameters), every " +
			"argument is checked against its `extends` bound (Is-membership — " +
			"lattice bounds, predicate refinements, disjunctions, negations, and " +
			"surfaces all answer uniformly), and the result is MEMOISED: one " +
			"instantiation per (schema, canonical arguments), so repeated " +
			"`Box of [Integer]` is the same type (`teq` true).",
		Examples: []string{
			`def Box gen [T] refine Record [value:T]`,
			`Box of [Integer] ; # record{value:Integer}`,
			`(Box of [Integer]) teq (Box of [Integer]) ; # true`,
			`Box of [] ; # ERROR arity_mismatch`,
		},
	})

	register(&Entry{
		Word:    "extends",
		Summary: "Bound a gen type parameter (inside gen [...]).",
		Description: "`gen [(T extends Comparable)]` constrains the parameter: every " +
			"instantiation argument must be a member of the bound — checked " +
			"loudly at `of` (constraint_violation). The bound is any type: a " +
			"lattice type (Number), a refinement ((Integer gt 0)), a disjunction, " +
			"a negation, or a surface (membership = explicit `exposes` " +
			"conformance). Only meaningful inside a gen parameter entry.",
		Examples: []string{
			`def Sorted gen [(T extends Number)] refine Record [items:[:T]]`,
			`Sorted of [Integer] ; # ok — Integer is a Number`,
			`Sorted of [String] ; # ERROR constraint_violation`,
		},
	})

	register(&Entry{
		Word:    "default",
		Summary: "Default a gen type parameter (inside gen [...]).",
		Description: "`gen [T (E default Error)]` lets `of` omit trailing arguments: " +
			"`Result of [Integer]` fills E with Error. Defaults are LAZY and may " +
			"reference earlier parameters (`gen [T (U default T)]`). Chains after " +
			"extends: `(T extends C default D)`.",
		Examples: []string{
			`def Result gen [T (E default Error)] refine Record [ok:T err:E]`,
			`Result of [Integer] ; # record{ok:Integer err:Error}`,
			`Result of [Integer String] ; # the default is overridable`,
		},
	})

	register(&Entry{
		Word:    "const",
		Summary: "Make a singleton type: a value whose TYPE has one inhabitant.",
		Description: "`const v` mints an interned singleton type under v's own type and " +
			"returns v tagged with it — `typeof (const 1)` renders as `1`. In a class " +
			"schema, `{kind:(const 'point')}` declares a field that can only ever hold " +
			"'point': the default is forced, the exact value is accepted at make/set, " +
			"anything else is a loud type error. Membership is same-base strict (1.0 is " +
			"not a member of (const 1)). Exemplars must be immutable (scalars, Lists, " +
			"Maps); NaN is rejected. With tor unions this gives discriminated records: " +
			"def Circle class {kind:(const 'circle'), r:0.0}.",
		Examples: []string{
			`def S class {kind:(const 'point') x:1}`,
			`make S {} ; # kind='point' — forced default`,
			`make S {kind:'other'} ; # ERROR — only 'point' inhabits the type`,
		},
	})

	register(&Entry{
		Word:    "base",
		Summary: "Return the zero/default value for the type of a value.",
		Description: "Consumes a value and returns the zero value for its type: 0 for integers, " +
			"0.0 for floats, empty string for strings, false for booleans, empty list for lists.",
	})

	register(&Entry{
		Word:    "tor",
		Summary: "Construct a disjunct (union) type from two values.",
		Description: "Returns a disjunct that matches either alternative. " +
			"Flattens nested disjuncts and applies carrier widening. " +
			"Use to build optional fields and union type literals " +
			"(e.g. `string tor none`).",
	})

	register(&Entry{
		Word:    "tand",
		Summary: "Combine two values by conjunction.",
		Description: "For two concrete maps, merges keys (unifying values for " +
			"keys present in both). For other shapes, returns the unification " +
			"of the two arguments. Errors if the values cannot be combined " +
			"(e.g. conflicting concrete values for the same key).",
	})

	register(&Entry{
		Word:    "tany",
		Summary: "Apply `tor` across a list, building a flattened disjunct.",
		Description: "Folds the list with `tor` semantics: every element becomes " +
			"an alternative of the resulting disjunct. Existing disjunct elements " +
			"are flattened. A single-element list returns that element unchanged. " +
			"Errors on an empty list.",
	})

	register(&Entry{
		Word:    "tall",
		Summary: "Apply `tand` across a list, folding via map-merge / unify.",
		Description: "Folds the list with `tand` semantics: concrete maps are " +
			"merged key-by-key (overlapping keys are unified); other shapes are " +
			"unified pairwise. A single-element list returns that element " +
			"unchanged. Errors on an empty list or unifiable failure.",
	})

	register(&Entry{
		Word:    "teq",
		Summary: "Strict type equality (lattice node identity, not subtype).",
		Description: "Returns true iff both args are types AND refer to the same " +
			"lattice node. Distinct from `is`, which is subtype membership: " +
			"`Integer is Number` is true, `Integer teq Number` is false. " +
			"Non-type arguments return false on either side.",
	})

	register(&Entry{
		Word:    "tpartial",
		Summary: "Wrap every field of a Record or class type in `T | None`.",
		Description: "Returns a new type where each field's value type is " +
			"replaced with the disjunct of itself and None. Idempotent — a " +
			"field already including None is unchanged. For class types, " +
			"inherited fields are flattened into the result.",
	})
}

func init() {
	register(&Entry{
		Word:        "is",
		Summary:     "Test whether a value is of a given type: `value is Type`.",
		Description: "True when the value conforms to (is an inhabitant of) the type — 5 is Integer, 'x' is String. Routes through the type's matching behaviour, so refine and predicate types are honoured.",
		Examples:    []string{`5 is Integer ; # => true`, `'x' is Integer ; # => false`},
	})
	register(&Entry{
		Word:    "as",
		Summary: "Dispatch a value as an ancestor type for ONE call: `value as Type`.",
		Description: "Returns the value UNCHANGED, carrying a match-time ascription: the next " +
			"signature match treats it as the ascribed type, then argument delivery strips it — " +
			"the handler or fn body receives the real value, so typeof/is/rendering/equality " +
			"never change. Upcast-only: the type must be an ancestor of the value's own type. " +
			"This is the delegation escape for subtype overrides of core words: inside an " +
			"anchored `set` override, `set k v (m as FlexMap)` dispatches the base FlexMap " +
			"overload — the override's own signature cannot match the widened view — with no " +
			"renamed shadow words and no retagging.",
		Examples: []string{`typeof (5 as Number) ; # => Integer`, `(5 as Number) add 1 ; # => 6`},
	})
	register(&Entry{
		Word:    "tis",
		Summary: "Nominal subtype test — `is` but walking only the lattice (Value.Parent).",
		Description: "Like `is`, but consults ONLY the type lattice: it reduces each " +
			"operand to the lattice node it denotes and asks whether the left node " +
			"sits on the right node's parent chain. It does NOT run a predicate, " +
			"refine, membership, or structural match — so it is tag-only. " +
			"`100 tis (Integer gt 10)` and `5 tis (Integer gt 10)` are both true " +
			"(base tag Integer), where `is` honours the predicate (true / false).",
		Examples: []string{`5 tis Integer ; # => true`, `5 tis Number ; # => true`, `Integer tis Number ; # => true`},
	})
	register(&Entry{
		Word:        "istype",
		Summary:     "Test whether a value is a type literal rather than a concrete value.",
		Description: "True for a bare type node like Integer or a user-defined type; false for concrete values like 5.",
		Examples:    []string{`istype Integer ; # => true`, `istype 5 ; # => false`},
	})
	register(&Entry{
		Word:    "behave",
		Summary: "Install a capability on a type: compare, canon, nodify, unify, truthy, deq, size, make.",
		Description: "`behave NAME fn` attaches a body the kernel runs whenever it dispatches that capability for the type in the fn's first param — how a user or external type opts into kernel operations instead of inheriting a default. " +
			"`compare` (fn [[T T] [Integer]]) drives cmp/sort/lt/gt; `canon` (fn [[T] [String]]) drives rendering; `nodify` (fn [[T] [Any]]) drives the Node projection behind jsonify; `unify` (fn [[T T] [T]]) drives type unification; " +
			"`truthy` (fn [[T] [Boolean]]) decides what a value means in a boolean position — `if`, the connectives, loop conditions; `deq` (fn [[T T] [Boolean]]) decides deep equality; `size` (fn [[T] [Integer]]) answers `size`; " +
			"`make` (fn [[Any] [T]]) is the type's CONSTRUCTOR, and takes its target from the RETURN type rather than a param — construction has no receiver, only a target and an arbitrary source. " +
			"Installs accumulate: a second `behave` on the same type adds a slot without dropping the first. Builtin types are refused — refine one first.",
		Examples: []string{
			`def Level (refine Integer) behave truthy/q (fn Level Boolean [a gt 5]) def lo:Level 3 if lo ['yes'] ['no'] ; # => 'no'`,
			`def Level (refine Integer) behave size/q (fn Level Integer [42]) def x:Level 3 size x ; # => 42`,
			`def Cel (refine Float) behave make/q (fn Float Cel [a sub 32.0 mul 0.5555]) make Cel 212.0 ; # => 99.99`,
		},
		Notes: []string{
			"The body sees the operands as `a` (and `b` for the two-argument slots).",
			"A slot is scoped to the type that installed it — a plain Integer keeps the kernel rule.",
			"Descendants inherit their branch's capability; the kernel walks the parent chain nearest-first.",
			"`make` runs ahead of every kernel construction path, so it also overrides the Ideal-kind default for object/record/table types — and `boru check` sees the slot from the `behave make` call on, so a construction after it is not validated against the declared schema the constructor replaces (NUR076). A construction before the call is, as the run's order has it.",
			"A `make` body that returns the wrong type is refused; one that returns the target's base type is reparented, as the kernel's own newtype construction does.",
		},
	})
	register(&Entry{
		Word:    "fnsig",
		Summary: "A function TYPE — a function minus its body.",
		Description: "Two forms, mirroring fn one slot shorter: the 2-arg pair form `fnsig Integer String`, " +
			"and the spec-list form `fnsig [[Integer] [String]]` for 2+ parameters, zero parameters, or an " +
			"overload set (a LIST input always selects the spec-list form, exactly as with fn). Bound to a " +
			"capitalised name it is a type dispatch ENFORCES: a parameter declared `g:IntToStr` admits only " +
			"functions whose shape satisfies it — parameters contravariant, returns covariant — where a bare " +
			"`Function` parameter admits any function at all. Also the selector targeted undef uses to remove " +
			"one overload: `undef name fnsig [[Number] [Number]]`.",
		Examples: []string{
			`typeof (fnsig Integer String) ; # => FunctionSignature`,
			`def T fnsig Integer String def f fn x:Integer String [convert String x] f/v is T ; # => true`,
			`def T fnsig Integer String def f fn x:String Integer [size x] f/v is T ; # => false — shape decides`,
		},
	})
	register(&Entry{
		Word:    "fnpred",
		Summary: "A predicate TYPE — a function minus its output.",
		Description: "The mirror of fnsig: a function TYPE is a function minus its BODY, a predicate type " +
			"is a function minus its OUTPUT, because a predicate's output is not a choice — it is membership. " +
			"Two forms, exactly as fnsig has: the 2-arg pair form `fnpred n:Integer [eq 0 (mod 2 n)]`, and the " +
			"spec-list form `fnpred [[n:Integer] [eq 0 (mod 2 n)]]` (a LIST input always selects the spec-list " +
			"form, as with fn). Bound to a capitalised name it is a type membership ENFORCES: `4 is Even`, and " +
			"a parameter or field declared `:Even` admits only values the body accepts. Two membership " +
			"conventions are supported and the body chooses: return a Boolean, or return the VALUE for a " +
			"member and `none` for a non-member. " +
			"Use it whenever you mean a membership test — it says so, where a bare capitalised `def` over a " +
			"fn body leaves the reader (and the engine) to infer it from the name's case.",
		Examples: []string{
			`def Even fnpred n:Integer [eq 0 (mod 2 n)] 4 is Even ; # => true`,
			`def Even fnpred n:Integer [eq 0 (mod 2 n)] 5 is Even ; # => false`,
			`def Positive fnpred n:Integer [if (n gt 0) [n] [None]] 0 is Positive ; # => false — none means non-member`,
		},
	})
	register(&Entry{
		Word:        "tnot",
		Summary:     "The negation of a type: the type of every value that is NOT of it.",
		Description: "`tnot Integer` is a Type/Negation matching anything that is not an Integer. Combine with type unions and intersections for precise constraints.",
		Examples:    []string{`typeof (tnot Integer) ; # => Negation`},
	})
	register(&Entry{
		Word:        "pathof",
		Summary:     "The lattice path of a value's type, as a list of names.",
		Description: "`pathof Integer` is [Scalar Number Integer] — the chain from the branch root down to the type. The structural complement of typeof.",
		Examples:    []string{`pathof Integer ; # => [Scalar Number Integer]`},
	})
}
