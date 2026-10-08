package modules

func init() {
	registerDocs("boru:debug", map[string]string{
		"tap":         "Print a value (formatted), then return it unchanged — the pipeline tap `print` can't be.",
		"label":       "The labelled tap: `<value> \"label\" Debug.label` prints \"label: value\" and returns the value.",
		"dump":        "Print a value with its type, then return it unchanged.",
		"assert":      "Raise assertion_failure with a message when the condition is false (`cond msg Debug.assert`).",
		"todo":        "A typed hole: always raise not_implemented with the given message (returns Never).",
		"parse":       "Parse boru source to its token/value list without running it.",
		"deps":        "The distinct word names a quoted body references. Deprecated (NUR063): canonical home boru:scry (`Scry.deps`); this frozen copy is removed in the first minor release after the one that ships boru:scry.",
		"explain":     "The full `describe` text for a word, returned as a String.",
		"sig":         "The signatures of a word as structured {args, returns} data. Deprecated (NUR063): canonical home boru:scry (`Scry.sig`); this frozen copy is removed in the first minor release after the one that ships boru:scry.",
		"body":        "The quoted body of a boru-defined word; `native` for a host word. Deprecated (NUR063): canonical home boru:scry (`Scry.body`); this frozen copy is removed in the first minor release after the one that ships boru:scry.",
		"watch":       "Print a name's current binding and return it (None if unbound).",
		"stack":       "A snapshot of the current data stack at the call site, as a List.",
		"disasm":      "Compile a quoted body to its StackForm disassembly (a String).",
		"heap":        "Go-runtime heap stats: {alloc, total-alloc, heap-objects, num-gc}.",
		"gc":          "Force a GC and report before/after heap deltas.",
		"step":        "Run a quoted body under interactive single-step control.",
		"break":       "A breakpoint: pause when a controller is attached; else a no-op.",
		"break-when":  "A conditional breakpoint: pause only when the condition is true.",
		"run-stepped": "Parse a source string and step it.",
		"widget":      "Construct a dashboard widget map from a title and a sample source string.",
		"dashboard":   "Render a one-shot snapshot of a list of widget maps to output.",
		"words":       "Every word actually dispatchable in this registry (live natives + defs). Deprecated (NUR063): canonical home boru:scry (`Scry.words`); this frozen copy is removed in the first minor release after the one that ships boru:scry.",
		"defs":        "Current def-bound names mapped to their active top binding. Deprecated (NUR063): canonical home boru:scry (`Scry.defs`); this frozen copy is removed in the first minor release after the one that ships boru:scry.",
		"modules":     "The native modules available to import. Deprecated (NUR063): canonical home boru:scry (`Scry.modules`); this frozen copy is removed in the first minor release after the one that ships boru:scry.",
		"sizeof":      "Estimated retained byte size of a value (deep walk).",
		"shape":       "Structural census of a value: counts by kind, node count, and max depth. Deprecated (NUR063): canonical home boru:scry (`Scry.shape`); this frozen copy is removed in the first minor release after the one that ships boru:scry.",
		"steps":       "Engine step count for a body — deterministic, clock-free perf signal.",
		"time":        "Run a body once, returning {result, elapsed-ms, steps}.",
		"bench":       "Run a body n times (`body n Debug.bench`), returning timing stats.",
		"trace":       "Run a body with step-by-step stack tracing printed to output.",
		"profile":     "Per-word step-count profile of a body, costliest first.",
	})

	// Several of these PASS THEIR VALUE THROUGH, which is what makes them
	// droppable into a pipeline and is invisible in a [Any] signature.
	// Results are from verified lang/spec/module-debug.tsv rows.
	registerExamples("boru:debug", map[string][]string{
		"tap": {
			`42 Debug.tap                                     ;# 42 — prints it AND returns it`,
			`;# So ` + "`(f x) Debug.tap g`" + ` inspects the middle of a pipeline`,
			`;# without changing what it computes.`,
		},
		"label": {`7 "answer" Debug.label                           ;# prints "answer: 7", returns 7`},
		"dump":  {`5 Debug.dump                                     ;# prints the value's full structure, returns it`},
		"assert": {
			`99 Debug.assert true "ok"                        ;# 99 — passes through when the condition holds`,
		},
		"parse":   {`size (Debug.parse "1 add 2")                     ;# 3 — the token list, nothing evaluated`},
		"disasm":  {`([1 add 2] Debug.disasm) is String               ;# the compiled bytecode, as text`},
		"explain": {`(Debug.explain "add") is String                  ;# how dispatch would resolve that word`},
		"sig":     {`("add" Debug.sig) typeof                        ;# List — the signatures, as data`},
		"deps":    {`size ([1 add 2 mul 3] Debug.deps)                ;# 2 — the words a body depends on`},
		"watch":   {`def k 5   ("k" Debug.watch)                      ;# 5 — report on every change to that binding`},
		"time":    {`Debug.time [ expensive-thing ]                   ;# the body's result, plus its elapsed time`},
		"bench":   {`Debug.bench 1000 [ 1 add 2 ]                     ;# run it N times, report the distribution`},
		"profile": {`Debug.profile [ expensive-thing ]                ;# per-word time for the body`},
		"steps":   {`Debug.steps [1 add 2]                            ;# every engine step, as data`},
		"stack":   {`(Debug.stack) typeof                            ;# List — the live operand stack`},
		"words":   {`Debug.words typeof                              ;# List — every registered word`},
		"defs":    {`Debug.defs typeof                               ;# Map — the live bindings`},
		"todo":    {`Debug.todo "handle the empty case"              ;# raises; a marker that cannot be forgotten`},
	})
}
