package core

import (
	"sort"
)

// frameStateWords are the words whose execution can install a binding in
// the CURRENT def scope, construct an inner fn (which reads the enclosing
// fn baseline), or run opaque dynamic code in scope. A fn body that
// contains none of them (scanning through code lists, parens and interp
// expressions, but not quoted data or already-built inner closures)
// provably creates no body-local defs and constructs no inner fn — so its
// frame needs neither the def-cleanup snapshot nor the fn baseline
// snapshot. The set is deliberately generous: an omission would over-skip
// and leak, so err toward keeping frame state (the closure/def/each tests
// exercise every entry). See buildFnBodyHandler and
// design/legacy/INTERPRETER-SPEED-PLAN.10.ignore #5.
var frameStateWords = map[string]bool{
	"def": true, "undef": true, // bind / unbind in scope
	"fn": true, "afn": true, // construct an inner fn (reads baseline)
	"do": true, "call": true, "eval": true, // run code in the current scope
	"var":    true,                                 // scoped temporaries desugar to def
	"word":   true,                                 // splice unevaluated code into the stream
	"module": true, "import": true, "export": true, // module-scope binding
	"usurp": true, "behave": true, // word / type-behavior modification
}

// bodyNeedsFrameState reports whether a fn body may install a body-local
// def or construct an inner fn during execution — i.e. whether its frame
// needs the two DefTable snapshots (fn baseline + def-cleanup). It is a
// conservative over-approximation: any frameStateWords occurrence, at any
// code depth, forces the snapshots. Reuses the vetted WalkBodyWords
// recursion (skips quoted data and nested closures).
//
// A bare body Word can HIDE a frameStateWord: a `word`-macro (an __SP
// splice binding, `def m word [fn …]`) splices its payload inline into
// THIS frame and runs it against the live stack, so a macro whose
// expansion constructs an inner fn or installs a def needs the baseline
// exactly as a literal `fn`/`def` would — but the body only shows the
// macro's name. So resolve each body Word against r: if it is bound to a
// splice, walk the macro's payload too (recursively, with a cycle guard).
// Macro-ness is judged at construction time, consistent with the rest of
// the frame-state analysis; a word unbound or non-macro here is treated as
// non-macro (the same assumption recursion's forward refs rely on).
func bodyNeedsFrameState(r *Registry, body []Value) bool {
	needs := false
	seen := map[string]bool{} // guards mutually-recursive macros
	var walk func([]Value)
	walk = func(toks []Value) {
		walkBodyTokens(toks, func(w WordInfo, _ Value) {
			if needs {
				return
			}
			if frameStateWords[w.Name] {
				needs = true
				return
			}
			if r == nil || seen[w.Name] {
				return
			}
			bound, ok := r.Defs.Top(w.Name)
			if !ok {
				return
			}
			info, ok := bound.Data.(SpliceInfo)
			if !ok {
				return
			}
			seen[w.Name] = true
			walk(SpliceExpand(info.Data))
		}, func(info SugarInfo, _ Value) {
			// A sugar marker steps as its bound role word (a `=>` marker
			// IS the afn construction) — judge it by the word the registry
			// would lower it to. An unbound role errors before it could
			// construct anything, so it needs no frame state.
			if needs {
				return
			}
			if name, bound := r.SugarWord(info.Kind); bound && frameStateWords[name] {
				needs = true
			}
		})
	}
	walk(body)
	return needs
}

// bodyReferencesArgs reports whether a fn body may read the per-call
// args list (the `args` word / `args.N` reach). Computed once at handler
// construction; when false — AND the body already passed the
// !bodyNeedsFrameState gate, which excludes every opaque-code word
// (do/call/eval/word/…) that could reach `args` dynamically — the
// handler pushes a shared empty list instead of copying the call's args
// into a fresh list per call (design/legacy/INTERPRETER-SPEED-PLAN.10.ignore #5).
// The WalkBodyWords token space is complete under that gate (it descends
// into code lists, parens, interp/XML expressions and Reach receivers,
// so `args.0` is seen), and macro splices are resolved recursively below.
//
// Same accepted gap as bodyNeedsFrameState: macro-ness is judged at
// construction, so a word unbound now and later rebound to a `word`-macro
// expanding to `args` would see the empty list — a visible empty `args`,
// never a silently wrong value (args lists are value-semantics ListPayload).
func bodyReferencesArgs(r *Registry, body []Value) bool {
	refs := false
	seen := map[string]bool{} // guards mutually-recursive macros
	var walk func([]Value)
	walk = func(toks []Value) {
		walkBodyTokens(toks, func(w WordInfo, _ Value) {
			if refs {
				return
			}
			if w.Name == "args" {
				refs = true
				return
			}
			if r == nil || seen[w.Name] {
				return
			}
			bound, ok := r.Defs.Top(w.Name)
			if !ok {
				return
			}
			info, ok := bound.Data.(SpliceInfo)
			if !ok {
				return
			}
			seen[w.Name] = true
			walk(SpliceExpand(info.Data))
		}, func(info SugarInfo, _ Value) {
			// Same role-word judgement as bodyNeedsFrameState: a marker
			// steps as its bound word, so it reads args iff that word is
			// `args` (no current role is, but the binding decides).
			if refs {
				return
			}
			if name, bound := r.SugarWord(info.Kind); bound && name == "args" {
				refs = true
			}
		})
	}
	walk(body)
	return refs
}

// WalkBodyWords recursively visits every bare Word in a fn body's
// value stream, invoking callback for each. Used by computeCaptures
// to enumerate the names a body references at construction time.
//
// Walks INTO: nested lists (auto-eval or quoted), paren-expr
// payloads, interpolated-string expression parts. Does NOT walk into
// quoted lists (they're data), nor into nested FnDefInfo payloads
// (those are inner closures with their own capture computation).
//
// The walker is strictly read-only — it does not mutate body or
// registry state.
func WalkBodyWords(body []Value, callback func(WordInfo, Value)) {
	walkBodyTokens(body, callback, nil)
}

// walkBodyTokens is WalkBodyWords plus an optional sugar-marker
// callback: sugarCB (when non-nil) fires for every Word/__SG marker
// the walk reaches, in addition to the word emissions. Frame-state
// analysis uses it to see the constructions a marker will lower to
// (a `=>` marker is an afn construction the word walk can no longer
// observe).
func walkBodyTokens(body []Value, callback func(WordInfo, Value), sugarCB func(SugarInfo, Value)) {
	for _, v := range body {
		walkBodyValue(v, callback, sugarCB)
	}
}

func walkBodyValue(v Value, callback func(WordInfo, Value), sugarCB func(SugarInfo, Value)) {
	// Quoted values are data — skip.
	if v.Quoted {
		return
	}
	// Bare Word: emit.
	if IsWord(v) {
		w, _ := AsWord(v)
		callback(w, v)
		return
	}
	// Sugar marker: report it to the marker callback, then walk the
	// value streams it carries (an Angle marker's gen-params head and
	// use-site type args re-enter the body when the engine lowers the
	// marker, so the words inside them are genuine body references).
	if info, ok := AsSugar(v); ok && IsSugar(v) {
		if sugarCB != nil {
			sugarCB(info, v)
		}
		if info.Head.Parent != nil {
			walkBodyValue(info.Head, callback, sugarCB)
		}
		for _, it := range info.Items {
			walkBodyValue(it, callback, sugarCB)
		}
		return
	}
	// Nested FnDefInfo: opaque. Its captures were resolved at its
	// own construction; don't descend.
	if _, ok := v.Data.(FnDefInfo); ok {
		return
	}
	// List payload: recurse.
	if v.Parent.Equal(TList) && v.Data != nil {
		lst, _ := AsList(v)
		for _, e := range lst.Slice() {
			walkBodyValue(e, callback, sugarCB)
		}
		return
	}
	// Paren-expr payload (stored inside map values when a paren
	// group appears as a data position): walk the inner tokens.
	if IsParenExpr(v) {
		toks, _ := AsParenExpr(v)
		for _, t := range toks {
			walkBodyValue(t, callback, sugarCB)
		}
		return
	}
	// Interpolated string: walk each expression part.
	if IsInterpString(v) {
		parts, _ := AsInterpString(v)
		for _, p := range parts {
			for _, t := range p.Expr {
				walkBodyValue(t, callback, sugarCB)
			}
		}
		return
	}
	// Interpolated XML literal: walk every ${...} expression in the
	// skeleton (attribute values and child holes, recursively) so a
	// closure over `<p>${x}</p>` captures `x`.
	if IsXmlInterp(v) {
		tmpl, _ := AsXmlInterp(v)
		walkXmlTmplExprs(tmpl, callback, sugarCB)
		return
	}
	// Map payload: walk each value (keys are strings, not Words).
	if v.Parent.Equal(TMap) && v.Data != nil {
		m, _ := AsMap(v)
		if m == nil {
			return
		}
		for _, key := range m.Keys() {
			mv, _ := m.Get(key)
			walkBodyValue(mv, callback, sugarCB)
		}
		return
	}
	// Reach (`.`/`!.` access, e.g. `e.code`, `m.a.(k)`): the receiver is a
	// value/word stream and a computed key is an expression. Walk both so a
	// name used ONLY as a reach receiver (`e1.code`) or inside a computed key
	// (`m.(k)`) is still seen — both for closure-capture analysis (a closure
	// over `x.field` must capture `x`) and for check-mode use-recording (the
	// receiver `def` is genuinely used). Literal bare-word keys (`.code`) are
	// field names, not references, and are NOT walked.
	if IsReach(v) {
		if info, err := AsReach(v); err == nil {
			for _, t := range info.Receiver {
				walkBodyValue(t, callback, sugarCB)
			}
			for _, seg := range info.Segments {
				for _, t := range seg.KeyExpr {
					walkBodyValue(t, callback, sugarCB)
				}
			}
		}
		return
	}
	// All other shapes (numbers, strings, booleans, atoms, type
	// literals, markers): nothing to capture.
}

// walkXmlTmplExprs walks every ${...} expression token in an XML
// interpolation skeleton — attribute values and child holes — recursing
// into nested child templates, so closure-capture analysis sees the word
// references inside a `<tag attr=${e}>${e2}</tag>` literal.
func walkXmlTmplExprs(t XmlTmpl, callback func(WordInfo, Value), sugarCB func(SugarInfo, Value)) {
	for _, a := range t.Attr {
		for _, p := range a.Parts {
			for _, tok := range p.Expr {
				walkBodyValue(tok, callback, sugarCB)
			}
		}
	}
	for _, c := range t.Cren {
		switch c.Kind {
		case XmlCrenExpr:
			for _, tok := range c.Expr {
				walkBodyValue(tok, callback, sugarCB)
			}
		case XmlCrenChild:
			if c.Child != nil {
				walkXmlTmplExprs(*c.Child, callback, sugarCB)
			}
		}
	}
}

// ComputeCaptures walks a single fn-sig body and returns the list of
// enclosing-fn-local bindings the body references. Returns nil at top
// level (no enclosing fn → no captures) or when no body Word resolves
// to an enclosing-fn local. Result is sorted by name for deterministic
// install order at dispatch.
//
// Capture rule: name is captured iff (1) the name is currently bound
// in r.Defs AND (2) Depth(name) > baseline[name] where baseline is the
// innermost TopFnBaseline. Names not bound (forward refs / recursion),
// names bound at module/global scope (Depth ≤ baseline), and the sig's
// own named params are all skipped.
func ComputeCaptures(r *Registry, sig *FnSig) []CapturedBinding {
	if r == nil {
		return nil
	}
	baseline := r.TopFnBaseline()
	if baseline == nil {
		return nil
	}
	paramNames := make(map[string]bool, len(sig.Params))
	for _, p := range sig.Params {
		if p.Name != "" {
			paramNames[p.Name] = true
		}
	}
	// Names bound by a `def NAME …` (or `var [[NAME …] …]`) INSIDE this body are
	// the body's OWN locals — the closure-body compile promotes them to frame
	// slots, so they are never captures, even though an analysis sub-engine run of
	// the body leaves them bound in r ABOVE the enclosing-fn baseline (which would
	// otherwise make the depth check below grab them). This is the each/closure
	// body-local-value-def leaf: an each body `[def j (cur get 0) j]` must capture
	// only `cur` (the genuine enclosing binding), not its own `j`.
	bodyLocals := map[string]bool{}
	CollectBodyLocalDefs(sig.Body(), bodyLocals)
	seen := map[string]Value{}
	WalkBodyWords(sig.Body(), func(w WordInfo, _ Value) {
		if w.Name == "" || paramNames[w.Name] || bodyLocals[w.Name] {
			return
		}
		if _, dup := seen[w.Name]; dup {
			return
		}
		v, ok := r.Defs.Top(w.Name)
		if !ok {
			return
		}
		if r.Defs.Depth(w.Name) <= baseline[w.Name] {
			return
		}
		seen[w.Name] = v
	})
	if len(seen) == 0 {
		return nil
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]CapturedBinding, len(names))
	for i, n := range names {
		out[i] = CapturedBinding{Name: n, Value: seen[n]}
	}
	return out
}

// collectBodyLocalDefs gathers the names a body binds for ITSELF — `def NAME …`
// at any non-closure depth, plus `var [[NAME …] …]` temporaries — into locals.
// These are frame-locals of the body being analysed, NOT captures (see
// ComputeCaptures). It recurses into list / paren tokens but NOT into a nested
// FnDefInfo (an inner closure owns its own locals + capture analysis), mirroring
// walkBodyValue's descent rules.
func CollectBodyLocalDefs(body []Value, locals map[string]bool) {
	for i := 0; i < len(body); i++ {
		v := body[i]
		if v.Quoted {
			continue
		}
		if _, isFn := v.Data.(FnDefInfo); isFn {
			continue // nested closure: its defs are its own
		}
		if w, err := AsWord(v); err == nil {
			switch w.Name {
			case "def":
				// def NAME … — NAME is the next bare-word token.
				if i+1 < len(body) {
					if nw, nerr := AsWord(body[i+1]); nerr == nil && nw.Name != "" {
						locals[nw.Name] = true
					}
				}
			case "var":
				// var [[NAME …] body] — the decl list's first element holds the
				// temporary names; they desugar to `def NAME` at run time.
				if i+1 < len(body) && body[i+1].Parent.Equal(TList) && body[i+1].Data != nil {
					if outer, lerr := AsList(body[i+1]); lerr == nil {
						elems := outer.Slice()
						if len(elems) > 0 && elems[0].Parent.Equal(TList) && elems[0].Data != nil {
							if decls, derr := AsList(elems[0]); derr == nil {
								for _, d := range decls.Slice() {
									if dw, dwe := AsWord(d); dwe == nil && dw.Name != "" {
										locals[dw.Name] = true
									}
								}
							}
						}
					}
				}
			}
		}
		if v.Parent.Equal(TList) && v.Data != nil {
			if lst, lerr := AsList(v); lerr == nil {
				CollectBodyLocalDefs(lst.Slice(), locals)
			}
		} else if IsParenExpr(v) {
			if toks, perr := AsParenExpr(v); perr == nil {
				CollectBodyLocalDefs(toks, locals)
			}
		}
	}
}

// MergeCaptures combines per-sig capture lists into a single
// deduplicated list. Multi-sig fns get one captures list at the
// FnDefInfo level; if the same name appears in two sigs we take the
// first (they came from the same Defs.Top at the same construction
// time, so the Values are identical anyway).
func MergeCaptures(perSig [][]CapturedBinding) []CapturedBinding {
	if len(perSig) == 0 {
		return nil
	}
	if len(perSig) == 1 {
		return perSig[0]
	}
	seen := map[string]Value{}
	for _, list := range perSig {
		for _, cb := range list {
			if _, dup := seen[cb.Name]; dup {
				continue
			}
			seen[cb.Name] = cb.Value
		}
	}
	if len(seen) == 0 {
		return nil
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]CapturedBinding, len(names))
	for i, n := range names {
		out[i] = CapturedBinding{Name: n, Value: seen[n]}
	}
	return out
}
