package core

// BodyHasReplayHazard reports whether a code body (a list or paren of tokens)
// carries a REGISTRY-MUTATING statement whose per-run replay the compiled
// lane cannot reproduce from the check pass's single run: a capitalised
// `def` / `var` (a type mint — per call on the interpreter, with a name-part
// reservation that outlives the frame, NUR167), a capitalised `undef` (the
// mirror mutation, retiring a minted node), or an `import` (the module-loaded
// record survives while the namespace binding is truncated). The compiler's
// stamp paths and every fn-unit admission ask it; a body it flags runs where
// the mutation is real — the interpreter, through the callback seam or the
// fallback. Recurses into nested lists and parens.
func BodyHasReplayHazard(v Value) bool {
	var toks []Value
	switch d := v.Data.(type) {
	case ListPayload:
		toks = d.Elems
	case ParenExprPayload:
		toks = d.Toks
	default:
		return false
	}
	for i, t := range toks {
		if w, ok := t.Data.(WordInfo); ok {
			switch w.Name {
			case "import":
				return true
			case "def", "var", "undef":
				if i+1 < len(toks) && IsCapitalisedName(BindNameToken(toks[i+1])) {
					return true
				}
			}
		}
		if BodyHasReplayHazard(t) {
			return true
		}
	}
	return false
}

// BindNameToken extracts the name a def/var/undef token binds when its
// operand token is a bare word or a quoted atom; "" otherwise (a computed
// name, which no static screen can read).
func BindNameToken(v Value) string {
	switch d := v.Data.(type) {
	case WordInfo:
		return d.Name
	case AtomPayload:
		return d.Name
	}
	return ""
}
