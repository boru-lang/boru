package modules

func init() {
	registerDocs("boru:scry", map[string]string{
		"words":   "Every word actually dispatchable in this registry (live natives + defs).",
		"defs":    "Current def-bound names mapped to their active top binding.",
		"modules": "The native modules available to import.",
		"sig":     "The signatures of a word as structured {args, returns} data.",
		"body":    "The quoted body of a boru-defined word; `native` for a host word.",
		"deps":    "The distinct word names a quoted body references.",
		"shape":   "Structural census of a value: counts by kind, node count, and max depth.",
	})

	registerExamples("boru:scry", map[string][]string{
		"words": {`Scry.words typeof                                ;# List — every dispatchable word`},
		"sig":   {`("add" Scry.sig) typeof                          ;# List — the signatures, as data`},
		"deps":  {`size ([1 add 2 mul 3] Scry.deps)                 ;# 2 — the words a body depends on`},
		"shape": {`(Scry.shape [1 2 [3]]) get "max-depth"           ;# 2 — the value's nesting`},
	})
}
