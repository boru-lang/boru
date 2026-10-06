package help

func init() {
	// SQL-style query DSL words. These live in the boru:query module and
	// are accessed via dot notation after `import "boru:query"` — e.g.
	// Query.from, Query.where, Query.select. They form a left-to-right
	// pipeline in forward form: `Query.from people Query.where [age gt
	// 18] Query.select [name age]`.

	register(&Entry{
		Word:    "select",
		Summary: "Start a query, projecting columns (boru:query).",
		Description: "The SQL-order entry word: `select [name age] from people …`. Seeds a new " +
			"lazy query with the projection columns; an empty list [] selects every column. " +
			"Supports renaming ([col alias]), casting ([cast col type]), and aggregate functions " +
			"(sum, avg, min, max, count). The query runs only when its result is printed, " +
			"iterated, or otherwise needs rows. Imported from boru:query; call as Query.select.",
	})

	register(&Entry{
		Word:    "from",
		Summary: "Set the source table of a query (boru:query).",
		Description: "Sets the table the preceding `select` reads from — required in every query. " +
			"Given a bare name it looks the table up in the context store (set via " +
			"`context set <name> <table>`); given a table value it uses that. Imported from " +
			"boru:query; call as Query.from.",
	})

	register(&Entry{
		Word:    "where",
		Summary: "Add a filter condition to a query (boru:query).",
		Description: "Filters rows matching the condition. Supports operators: eq, neq, lt, gt, " +
			"lte, gte, like, in, between, is null, is not null, and, or, not. Imported from " +
			"boru:query; call as Query.where.",
	})

	register(&Entry{
		Word:    "order",
		Summary: "Specify sort order for query results (boru:query).",
		Description: "Orders query results by the specified columns. Use [col desc] for descending. " +
			"Imported from boru:query; call as Query.order.",
	})

	register(&Entry{
		Word:        "limit",
		Summary:     "Limit the number of query results (boru:query).",
		Description: "Restricts the query to return at most n rows. Imported from boru:query; call as Query.limit.",
	})

	register(&Entry{
		Word:        "offset",
		Summary:     "Skip rows in query results (boru:query).",
		Description: "Skips the first n rows before returning results. Imported from boru:query; call as Query.offset.",
	})

	register(&Entry{
		Word:        "distinct",
		Summary:     "Remove duplicate rows from query results (boru:query).",
		Description: "Adds SELECT DISTINCT to the query. Imported from boru:query; call as Query.distinct.",
	})

	register(&Entry{
		Word:        "group",
		Summary:     "Group query results by columns (boru:query).",
		Description: "Groups rows by the specified columns for aggregate queries. Imported from boru:query; call as Query.group.",
	})

	register(&Entry{
		Word:    "having",
		Summary: "Filter grouped query results (boru:query).",
		Description: "Filters groups after GROUP BY, like WHERE but for aggregated values. " +
			"Imported from boru:query; call as Query.having.",
	})

	register(&Entry{
		Word:    "join",
		Summary: "Join another table into a query (boru:query).",
		Description: "Adds a JOIN against the named table; pair with Query.on (ON condition) or " +
			"Query.using (shared columns). Variants: Query.join / Query.innerjoin (inner), " +
			"Query.leftjoin (left outer), Query.crossjoin (cross). Imported from boru:query.",
	})

	register(&Entry{
		Word:    "union",
		Summary: "Combine two queries with a set operation (boru:query).",
		Description: "Appends a set operation against a right-hand query: Query.union (distinct), " +
			"Query.unionall (keep duplicates), Query.intersect, Query.except. The right-hand " +
			"query is typically a parenthesized sub-pipeline. Imported from boru:query.",
	})

	register(&Entry{
		Word:    "unify",
		Summary: "Unify two values using structural pattern matching.",
		Description: "Attempts to unify two values. Pushes the unified value and a boolean. " +
			"Returns true if they match structurally.",
	})

	register(&Entry{
		Word:    "module",
		Summary: "Define a module with exported words.",
		Description: "Creates a named module. The list is evaluated in an isolated scope and " +
			"exported words become available under the module name.",
		Examples: []string{
			`import module [export "Utils" {greet:greet/v}] ; # inline module`,
		},
	})

	register(&Entry{
		Word:    "import",
		Summary: "Import a module or data file.",
		Description: "Loads a file as a module or data. File paths must start with /, ./ or ../. " +
			"For .boru files, executes as an isolated module and installs exports. " +
			"For .json/.jsonic files, pushes parsed data onto the stack. " +
			"For .csv/.tsv files, loads data as a table. " +
			"Use a list argument to rename imports (not supported for data files).",
		Examples: []string{
			`import "boru:math-util" ; # native module → math.* namespace`,
			`import "./lib.boru" ; # sibling file's exports`,
			`import [Orig Alias] "./lib.boru" ; # rename on import`,
		},
	})

	register(&Entry{
		Word:    "export",
		Summary: "Publish a namespace from a module.",
		Description: "Inside a module body, `export \"Name\" {key: value …}` publishes " +
			"a namespace reachable as Name.key after import. Export functions with the " +
			"/v ref modifier so the map doesn't dispatch them. At the top level (running " +
			"a file directly) export is a no-op, so one file can both run standalone and " +
			"export when imported.",
		Examples: []string{
			`export "Utils" {pi:pi greet:greet/v} ; # value bare, fn with /v`,
		},
	})

	register(&Entry{
		Word:    "each",
		Summary: "Map a code body over each element of a list, or each value of a map.",
		Description: "Runs the body once per element with the element on the stack, " +
			"collecting each body's result into a new list. The body must leave a value; " +
			"for a side-effecting loop that collects nothing, use for-each. " +
			"Over a map it transforms each value and keeps the keys (mapValues); a Function " +
			"callback is handed the value too, or the whole entry as a KeyVal {k v i n} when " +
			"its param is typed KeyVal, so it can use the key/index/total.",
		Examples: []string{
			`[1 2 3] each [dup mul] ; # => [1 4 9]`,
			`iota 4 each [1 add] ; # => [1 2 3 4]`,
			`{a:1 b:2} each [mul 10] ; # => {a:10 b:20}  (map: values transformed, keys kept)`,
		},
	})

	register(&Entry{
		Word:    "for-each",
		Summary: "Run a code body over each element (or map entry) for side effects.",
		Description: "Like each, but discards every body result and produces nothing. " +
			"The body may leave the stack empty (no sentinel needed) — use it for " +
			"mutating loops that don't collect a value. Works over a list or a map " +
			"(entries in insertion order; a KeyVal-typed callback param gets the entry).",
		Examples: []string{
			`[1 2 3] for-each [print] ; # prints 1,2,3; leaves nothing`,
			`{a:1 b:2} for-each [print] ; # prints the values 1,2`,
		},
	})

	register(&Entry{
		Word:    "fold",
		Summary: "Reduce a list (or a map's values) to a single value with an accumulator.",
		Description: "Runs the body with the accumulator and each element, threading the " +
			"result forward. With an initial value: `init fold [body] data`. Without one, " +
			"the first element seeds the accumulator: `fold [body] data`. " +
			"Body stack order: the accumulator is pushed first and the current ELEMENT " +
			"sits on top of it, so a two-arg word in the body sees (element, accumulator) " +
			"as (top, deeper) — `0 fold [sub] [10]` is acc-minus-element = -10, and " +
			"`[] fold [push] data` pushes each element onto the accumulator list. " +
			"Over a map it reduces the values; a Function gets (accumulator, value), or " +
			"(accumulator, KeyVal) when its entry param is typed KeyVal.",
		Examples: []string{
			`0 fold [add] [1 2 3 4] ; # => 10  (with initial value)`,
			`fold [add] [1 2 3] ; # => 6   (first element seeds it)`,
			`[] fold [push] [1 2 3] ; # => [1 2 3]  (element on top, acc beneath)`,
			`fold [add] {a:1 b:2 c:3} 0 ; # => 6   (map: sum the values)`,
		},
	})

	register(&Entry{
		Word:    "scan",
		Summary: "Running (prefix) fold: every intermediate accumulator, as a list or map.",
		Description: "Like fold, but returns each successive accumulator instead of only the " +
			"final one — a running total. The first element seeds the accumulator and is the " +
			"first output; each later step runs the body with (accumulator, element) — same " +
			"stack order as fold — and records the new accumulator. scan's last element " +
			"equals fold's result. Unlike fold there is no init form: scan always seeds from " +
			"the first element (empty input gives an empty result). Over a map it scans the " +
			"values, keeping the keys; a Function gets (accumulator, value), or (accumulator, " +
			"KeyVal) when its entry param is typed KeyVal.",
		Examples: []string{
			`scan [add] [1 2 3] ; # => [1 3 6]   (running sums)`,
			`scan [mul] [1 2 3 4] ; # => [1 2 6 24]  (running products)`,
			`{a:1 b:2 c:3} scan [add] ; # => {a:1 b:3 c:6}  (map: running fold over values)`,
		},
	})

	register(&Entry{
		Word:    "filter",
		Summary: "Keep the elements (or map entries) for which a predicate holds.",
		Description: "Three predicate forms. Quotation: `filter [body] xs` runs the body " +
			"once per element with the element on the stack (like each/fold) and keeps the " +
			"elements whose result is Boolean true — a non-Boolean result is an error, not " +
			"a silent drop. Reach lens: `filter $.active xs` keeps elements whose field is " +
			"true. Function: `filter (pred) xs` hands the callback the element (over a map " +
			"the value, or the KeyVal {k v i n} a KeyVal-typed param asks for). All three " +
			"forms keep the container shape — a list filters to a list, a map filters by " +
			"value to a map.",
		Examples: []string{
			`filter [2 gt] [1 2 3 4] ; # => [3 4]`,
			`filter $.active accounts ; # keep where .active is true`,
			`filter ([p:Integer] => [p gt 3]) [1 2 3 4 5] ; # => [4 5]`,
			`{a:1 b:5 c:3} filter [gt 2] ; # => {b:5 c:3}  (map: keep by value)`,
		},
	})

	register(&Entry{
		Word:    "walk",
		Summary: "Traverse any data structure, firing descent/ascent hooks on every node (visit-only).",
		Description: "`walk <options> <data> <descend> [<ascend>]` visits every node — root, " +
			"containers, and leaf scalars — of any value: lists, maps, XML, objects, arrays, " +
			"stores, tables, and user-defined types (descended generically via the IdealConverter " +
			"projection). `mode: \"depth\"` (default) fires descend in pre-order and ascend in " +
			"post-order; `mode: \"breadth\"` descends level-by-level and replays ascend in reverse " +
			"visitation order. Each hook receives a map `{key value path parent depth}`; the root " +
			"has key=None, path=\"\", parent=None, depth=0. Paths are dot-joined keys (list/array " +
			"indices stringified). walk is VISIT-ONLY: hooks run for side effects, their results are " +
			"ignored, and walk returns its input unchanged — accumulate by mutating a captured " +
			"FlexList. A hook is a lambda `(m:Any => …)` (reads `m.value`/`m.path`) or a quotation " +
			"`[…]` (the payload map arrives on the stack). Distinct from StructUtil.walk. Assumes " +
			"acyclic data.",
		Examples: []string{
			`def acc (flex [])  walk {mode: "depth"} {a:1 b:[2 3]} (m:Any => [acc (m.path) append])  acc`,
			`walk {mode:"breadth"} {a:1 b:{c:2}} (m:Any => [m.path print]) ; # print paths, level order`,
			`walk {mode:"depth"} {a:{x:1}} (m:Any => [m.path print]) (m:Any => [m.path print]) ; # pre then post order`,
		},
	})
}

func init() {
	register(&Entry{
		Word:        "list",
		Summary:     "List records from a resource, or filter a record set (CRUD read-many).",
		Description: "The read-many verb over a Resource entity or a list of records; with an options map it filters. Part of the resource/API access vocabulary alongside create / load / update / remove.",
	})
	register(&Entry{
		Word:        "create",
		Summary:     "Create a record in a resource (the CRUD create verb).",
		Description: "Inserts a new record into a Resource entity (or via an API pattern), returning the created record.",
	})
	register(&Entry{
		Word:        "load",
		Summary:     "Load a single record from a resource (the CRUD read verb).",
		Description: "Fetches one record from a Resource entity or API by its key — the read counterpart of create.",
	})
	register(&Entry{
		Word:        "update",
		Summary:     "Update a record in a resource (the CRUD update verb).",
		Description: "Writes changes to an existing record in a Resource entity or API.",
	})
	register(&Entry{
		Word:        "remove",
		Summary:     "Remove a record from a resource (the CRUD delete verb).",
		Description: "Deletes a record from a Resource entity or API.",
	})
	register(&Entry{
		Word:        "between",
		Summary:     "Query range predicate: `col between lo hi` (inclusive).",
		Description: "Used inside the where clause of a query pipeline; compiles to SQL col BETWEEN lo AND hi.",
	})
	register(&Entry{
		Word:        "outer",
		Summary:     "Outer product: apply a body to every pair drawn from two lists.",
		Description: "`outer [body] listB listA` builds the matrix of body results across all pairs (listA down the rows, listB across the columns).",
		Examples:    []string{`outer [mul] [1 2] [3 4] ; # => [[3 4] [6 8]]`},
	})
	register(&Entry{
		Word:        "inner",
		Summary:     "Generalized inner (matrix) product of two lists.",
		Description: "`inner [combine] [product] listB listA` pairs elements with product then reduces them with combine — the dot-product generalisation (combine=add, product=mul gives the usual dot product).",
	})
}
