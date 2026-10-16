package modules

func init() {
	registerDocs("boru:vm", map[string]string{
		"run":         "Run code in a sandboxed sub-engine, returning the result.",
		"run-sandbox": "Run code under the 'sandbox' profile (pure computation).",
		"run-compute": "Run code under the 'compute' profile (arithmetic).",
		"run-with":    "Run code under an explicit policy supplied as a data map.",
		"parse":       "Parse boru source to a quoted token list without running it (loud parse_error on malformed source).",
		"check":       "Static type-check source without running it, returning a { ok errors warnings diagnostics } report.",
	})

	// The four run variants differ only in the POLICY they run under,
	// which no signature of [String] can show — and picking the wrong one
	// is the mistake that matters here.
	registerExamples("boru:vm", map[string][]string{
		"run":         {`Vm.run "1 add 2"                                ;# 3 — the caller's own policy`},
		"run-sandbox": {`Vm.run-sandbox "5 mul 7"                        ;# 35 — no capabilities at all`},
		"run-compute": {`Vm.run-compute "1 add 2"                        ;# 3 — arithmetic, no I/O`},
		"run-with": {
			`{scopes: {engine: {words: {default: "allow"}}}} "10 sub 3" Vm.run-with   ;# 7`,
			`;# The policy is the FIRST operand and the source the second.`,
		},
		"check": {
			`(Vm.check "1 add 2").ok                          ;# true — static check, nothing runs`,
			`(Vm.check "totally-undefined-word").ok           ;# false`,
			`((Vm.check "totally-undefined-word").diagnostics.0).code    ;# 'undefined_word'`,
			`(Vm.check "1 add 2  def unused 99").warnings     ;# 1 — warnings are counted separately`,
		},
		"parse": {
			`size (Vm.parse "1 add 2")                        ;# 3 — the token list, unevaluated`,
		},
	})
}
