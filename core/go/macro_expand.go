package core

import (
	"fmt"
	"strings"
)

// Macro expansion (design/legacy/MACROS-PHASE1.10.ignore §6). A macro is an FnDef flagged
// Macro=true whose every param is FormArgs raw-capture. At dispatch the
// operands are captured unevaluated, the template body is run, the returned
// token list is walked to resolve `unquote`/`splice`, and the result is
// spliced into the call site via an __SP marker that re-steps the tape.
//
// Model A (the plan's): the template is data (a `quote [ … ]` list); the
// expander walks it resolving the escapes — it is NOT live `unquote`/`splice`
// words run during the body. The walk recurses into List, ParenExpr, AND Map
// values (the §3.1 map channel).

// execMacro is the dispatch entry: the macro FnDef value sits at valIdx with
// its operands ahead on the tape (still raw — the macro branch fires before
// preEvalParens). It captures `arity` operand forms, expands, and splices the
// expansion in place of the `mac operand…` span.
func (e *Engine) execMacro(valIdx int, fnDef *FnDefInfo) error {
	if len(fnDef.Signatures) == 0 {
		return fmt.Errorf("macro %s: no signature", fnDef.Name)
	}
	sig := &fnDef.Signatures[0]
	arity := len(sig.Params)

	operands, endIdx, err := e.scanMacroOperands(fnDef, arity, valIdx)
	if err != nil {
		return err
	}
	// Memoize on (name + operand canon): the expansion is deterministic, so a
	// macro re-applied to the same operand forms (e.g. in a loop) expands once
	// and re-splices the cached tokens. See design/legacy/MACROS-PHASE1.10.ignore §8.
	key := macroOperandsKey(fnDef.Name, operands)
	expanded, ok := e.Registry.macroCacheGet(key)
	if !ok {
		expanded, err = expandMacroWith(e.Registry, fnDef, operands)
		if err != nil {
			return err
		}
		e.Registry.macroCachePut(key, expanded)
	}
	// Replace the whole `mac operand…` span (valIdx..endIdx) with one __SP
	// marker; e.pointer == valIdx is left unchanged so the Run loop steps the
	// marker next, splicing the expansion onto the live tape and re-stepping.
	e.Tape.Splice(valIdx, endIdx-valIdx+1, NewSplice(NewList(expanded)))
	return nil
}

// macroOperandsKey builds the expansion-cache key from the macro name and the
// canon of each operand form — two calls with the same name and operand forms
// share an expansion, regardless of where they appear.
func macroOperandsKey(name string, operands []Value) string {
	var b strings.Builder
	b.WriteString(name)
	for _, op := range operands {
		b.WriteByte(0)
		b.WriteString(CanonValue(op))
	}
	return b.String()
}

// scanMacroOperands grabs the next `arity` operand FORMS after the macro value
// at valIdx — each a single tape value (a Word, literal, ParenExpr, List, Map,
// or Reach), stopping at a structural boundary. Returns the operands and the
// index of the last one. Errors if fewer than `arity` are available (a macro
// used without enough operands — including "as a bare value").
func (e *Engine) scanMacroOperands(fnDef *FnDefInfo, arity, valIdx int) ([]Value, int, error) {
	operands := make([]Value, 0, arity)
	idx := valIdx + 1
	last := valIdx
	for len(operands) < arity && idx < e.Tape.Len() {
		t := e.Tape.At(idx)
		// A RAW paren group is one operand FORM: fold the balanced span
		// into a ParenExpr — the same shape a group reaches a macro in
		// when the PRECEDING statement's parked forward folded it during
		// collection. A structurally-dispatched preceding statement
		// (def's keyword forms) leaves the group as markers, and the
		// macro must not depend on which path the previous statement
		// took.
		if IsOpenParen(t) {
			depth := 1
			j := idx + 1
			var inner []Value
			for ; j < e.Tape.Len(); j++ {
				v := e.Tape.At(j)
				if IsOpenParen(v) {
					depth++
				} else if IsCloseParen(v) {
					depth--
					if depth == 0 {
						break
					}
				}
				inner = append(inner, v)
			}
			if depth != 0 {
				break // unbalanced — treat as a structural boundary
			}
			pe := NewParenExpr(inner)
			pe.pos = t.pos
			operands = append(operands, pe)
			last = j
			idx = j + 1
			continue
		}
		if IsEnd(t) || IsCloseParen(t) || IsForward(t) ||
			t.Parent.ConformsTo(TMark) || t.Parent.ConformsTo(TMove) ||
			t.Parent.ConformsTo(TInternal) || t.Parent.ConformsTo(TReturnCheck) {
			break
		}
		operands = append(operands, t)
		last = idx
		idx++
	}
	if len(operands) < arity {
		return nil, 0, &BoruError{
			Code: "macro_error",
			Detail: fmt.Sprintf("macro %s expects %d operand(s), got %d",
				fnDef.Name, arity, len(operands)),
		}
	}
	return operands, last, nil
}

// ExpandMacroWith is the exported entry to expandMacroWith for host packages
// that hold a macro FnDef directly (rather than by name) — the minilang
// boru compile-hook path expands the `compile_<kind>` macro against the
// literal src and opts. Returns the expanded token list.
func ExpandMacroWith(r *Registry, fnDef *FnDefInfo, operands []Value) ([]Value, error) {
	return expandMacroWith(r, fnDef, operands)
}

// expandMacroWith runs a macro's template body against the given raw operands
// and returns the expanded token list. The macro scope (params + captures +
// any body-locals) stays live across the template walk so `unquote`/`splice`
// resolve against it, then is torn down. Shared by execMacro and macroexpand.
func expandMacroWith(r *Registry, fnDef *FnDefInfo, operands []Value) ([]Value, error) {
	sig := &fnDef.Signatures[0]

	// Bind params to their raw operand forms. `bindings` (operand forms,
	// inert) is the fast path for `unquote <paramName>`; the same forms are
	// installed (quoted) in the def scope so an `unquote (expr)` referencing a
	// param resolves too. Body-locals created while running the template (e.g.
	// `def g (gensym)`) leak into the scope and stay live for the walk.
	// A CONTENT-preserving snapshot (SnapshotEntries), not the depth-based
	// Defs.Snapshot: the depth restore can only TRUNCATE, so a template that
	// ran `undef` on a pre-existing name — or an overlapping redefinition —
	// escaped the region while the ledger bracket below suppressed its note
	// (Codex P2, PR #418). The entries restore is IN PLACE and gen-guarded —
	// a table SWAP (RestoreBindings) breaks every reference the enclosing
	// pass holds across this nested region, measured as ledgered defs
	// vanishing from the post-pass registry.
	snap := r.Defs.SnapshotEntries()
	// Everything installed from here to the restore calls below — a template
	// body-local like `def t (gensym)`, and anything an `unquote (expr)` runs —
	// is torn down by that restore, so the bind ledger must not record it: the
	// pass leaves no such binding, and the live-depth oracle read the recorded
	// installs as phantom depth (the gensym-temp class, 8 of its 9 mismatches
	// across edge-quote-3.tsv and macro.tsv).
	defer r.Check.SuppressBindLedger()()
	bindings := make(map[string]Value, len(sig.Params))
	for i, p := range sig.Params {
		if i >= len(operands) || p.Name == "" {
			continue
		}
		bindings[p.Name] = operands[i]
		q := operands[i]
		q.Quoted = true
		InstallFrameBinding(r, p.Name, q)
	}
	for _, cb := range fnDef.Captured {
		InstallFrameBinding(r, cb.Name, cb.Value)
	}

	// Run the template body → the template token list (its last result).
	body := make([]Value, len(sig.Body()))
	copy(body, sig.Body())
	res, runErr := New(r).Run(body)
	if runErr != nil {
		r.Defs.RestoreEntriesSnapshot(snap)
		return nil, fmt.Errorf("macro %s: template: %w", fnDef.Name, runErr)
	}
	if len(res) == 0 {
		r.Defs.RestoreEntriesSnapshot(snap)
		return nil, &BoruError{Code: "macro_error",
			Detail: fmt.Sprintf("macro %s: template produced no value", fnDef.Name)}
	}
	template := res[len(res)-1]
	var tmpl []Value
	if template.Parent.Equal(TList) && IsConcrete(template) {
		l, _ := AsList(template)
		tmpl = l.Slice()
	} else {
		tmpl = []Value{template}
	}

	// Automatic hygiene (design/MACROS.8.md §5.2 #1): rename every
	// template-origin `def` binder to a fresh gensym, consistently across the
	// template, so an introduced name can't capture a same-named use-site
	// variable. User-origin names — those supplied through unquote/splice,
	// including the `def unquote <name>` escape — are NOT in this set and pass
	// through unchanged, so a macro that intentionally binds a user-visible
	// name still can. (Free-word capture-pinning (#2) and DefCleanup teardown
	// are refinements — see MACROS-PHASE1.10.md.)
	renames := map[string]string{}
	binders := map[string]bool{}
	collectTemplateBinders(tmpl, binders)
	for name := range binders {
		renames[name] = r.NextGensym()
	}

	expanded, expErr := expandTemplate(r, tmpl, bindings, renames)
	r.Defs.RestoreEntriesSnapshot(snap)
	if expErr != nil {
		return nil, fmt.Errorf("macro %s: %w", fnDef.Name, expErr)
	}
	return expanded, nil
}

// collectTemplateBinders gathers the names a template binds with a literal
// `def <name>` (template-origin binders), recursing into nested containers. A
// `def unquote …` / `def splice …` name is user-controlled and excluded, so it
// is never auto-renamed.
func collectTemplateBinders(toks []Value, set map[string]bool) {
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if IsWord(t) {
			w, _ := AsWord(t)
			if w.Name == "def" && i+1 < len(toks) && IsWord(toks[i+1]) {
				nm, _ := AsWord(toks[i+1])
				if nm.Name != "unquote" && nm.Name != "splice" {
					set[nm.Name] = true
				}
			}
		}
		collectBindersNested(t, set)
	}
}

// collectBindersNested recurses collectTemplateBinders into a container value.
func collectBindersNested(t Value, set map[string]bool) {
	switch {
	case t.Parent.Equal(TList) && IsConcrete(t):
		l, _ := AsList(t)
		collectTemplateBinders(l.Slice(), set)
	case IsParenExpr(t):
		pt, _ := AsParenExpr(t)
		collectTemplateBinders(pt, set)
	case t.Parent.Equal(TMap) && IsConcrete(t):
		m, _ := AsMap(t)
		if m == nil {
			return
		}
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			collectBindersNested(v, set)
		}
	}
}

// expandTemplate walks a template token list, resolving `unquote x` (insert one
// grouped node) and `splice xs` (flatten a list's elements), and recursing into
// nested List / ParenExpr / Map values. Bare tokens pass through as code.
func expandTemplate(r *Registry, toks []Value, bindings map[string]Value, renames map[string]string) ([]Value, error) {
	out := make([]Value, 0, len(toks))
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if IsWord(t) {
			w, _ := AsWord(t)
			if w.Name == "unquote" || w.Name == "splice" {
				if i+1 >= len(toks) {
					return nil, &BoruError{Code: "macro_error",
						Detail: w.Name + ": missing operand in template"}
				}
				// The escape's operand is USER-origin: resolved against the
				// macro scope, NOT subject to hygiene renaming.
				val, err := resolveTemplateEscape(r, toks[i+1], bindings)
				if err != nil {
					return nil, err
				}
				i++ // consume the escape's operand
				if w.Name == "splice" {
					if !val.Parent.Equal(TList) || !IsConcrete(val) {
						return nil, &BoruError{Code: "macro_error",
							Detail: "splice: operand did not evaluate to a list"}
					}
					l, _ := AsList(val)
					out = append(out, l.Slice()...)
				} else {
					out = append(out, val)
				}
				continue
			}
			// A template-origin literal Word that names an auto-renamed
			// binder → its fresh gensym (hygiene #1).
			if g, ok := renames[w.Name]; ok {
				gw := NewWord(g)
				gw.pos = t.pos
				out = append(out, gw)
				continue
			}
		}
		nested, err := expandNested(r, t, bindings, renames)
		if err != nil {
			return nil, err
		}
		out = append(out, nested)
	}
	return out, nil
}

// resolveTemplateEscape resolves the operand of an `unquote`/`splice`. A bare
// param name maps directly to its raw operand form (fast path, no eval); any
// other operand (a paren / nested expr / body-local word) is evaluated in the
// live macro scope.
//
// An ATOM result becomes a WORD: a name produced for splicing (a `gensym`, or
// any name value) must be an identifier in the generated code, not inert atom
// data. `def unquote g` then binds the name and `if unquote g` resolves it —
// whereas a spliced atom would be truthy data in value position.
func resolveTemplateEscape(r *Registry, operand Value, bindings map[string]Value) (Value, error) {
	if IsWord(operand) {
		w, _ := AsWord(operand)
		if form, ok := bindings[w.Name]; ok {
			return asTemplateNode(form), nil
		}
	}
	res, err := New(r).Run([]Value{operand})
	if err != nil {
		return Value{}, err
	}
	if len(res) == 0 {
		return Value{}, &BoruError{Code: "macro_error",
			Detail: "unquote/splice operand produced no value"}
	}
	return asTemplateNode(res[len(res)-1]), nil
}

// asTemplateNode normalizes a resolved escape value for insertion into the
// expansion: an Atom (a name — e.g. a gensym) becomes a Word so it acts as a
// code identifier; everything else passes through unchanged.
func asTemplateNode(v Value) Value {
	if IsAtom(v) {
		name, _ := AsAtom(v)
		w := NewWord(name)
		w.pos = v.pos
		return w
	}
	return v
}

// expandNested recurses the escape walk into container values (a template may
// embed unquote/splice inside a list, a paren, or a map value — §3.1).
func expandNested(r *Registry, t Value, bindings map[string]Value, renames map[string]string) (Value, error) {
	switch {
	case t.Parent.Equal(TList) && IsConcrete(t):
		l, _ := AsList(t)
		inner, err := expandTemplate(r, l.Slice(), bindings, renames)
		if err != nil {
			return Value{}, err
		}
		nl := NewList(inner)
		nl.Quoted = t.Quoted
		return nl, nil
	case IsParenExpr(t):
		toks, _ := AsParenExpr(t)
		inner, err := expandTemplate(r, toks, bindings, renames)
		if err != nil {
			return Value{}, err
		}
		return NewParenExpr(inner), nil
	case t.Parent.Equal(TMap) && IsConcrete(t):
		m, _ := AsMap(t)
		if m == nil {
			return t, nil
		}
		out := NewOrderedMap()
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			rv, err := expandNested(r, v, bindings, renames)
			if err != nil {
				return Value{}, err
			}
			out.Set(k, rv)
		}
		return NewMap(out), nil
	default:
		return t, nil
	}
}

// ExpandMacroForm expands a `(mac operand…)` form and returns the resulting
// token list WITHOUT splicing or running it — the `macroexpand` introspection
// word. form is the captured raw call (a ParenExpr or List).
func ExpandMacroForm(r *Registry, form Value) ([]Value, error) {
	var toks []Value
	switch {
	case IsParenExpr(form):
		toks, _ = AsParenExpr(form)
	case form.Parent.Equal(TList) && IsConcrete(form):
		l, _ := AsList(form)
		toks = l.Slice()
	default:
		return nil, &BoruError{Code: "macroexpand_error",
			Detail: "macroexpand: expected a (macro operand…) form"}
	}
	if len(toks) == 0 || !IsWord(toks[0]) {
		return nil, &BoruError{Code: "macroexpand_error",
			Detail: "macroexpand: form must start with a macro word"}
	}
	name, _ := AsWord(toks[0])
	bound, ok := r.Defs.Top(name.Name)
	if !ok {
		return nil, &BoruError{Code: "macroexpand_error",
			Detail: "macroexpand: undefined macro " + name.Name}
	}
	fnDef, ok := bound.Data.(FnDefInfo)
	if !ok || !fnDef.Macro {
		return nil, &BoruError{Code: "macroexpand_error",
			Detail: "macroexpand: " + name.Name + " is not a macro"}
	}
	first, err := expandMacroWith(r, &fnDef, toks[1:])
	if err != nil {
		return nil, err
	}
	// Fully expand: a macro whose expansion calls another macro is expanded
	// too (macroexpand-all), matching what the runtime would do as the
	// spliced tokens re-step. Bounded to catch a runaway recursive macro.
	return expandAllMacros(r, first, 0)
}

// maxMacroExpandDepth bounds recursive macroexpand-all so a macro that expands
// to (a call to) itself fails loudly instead of looping forever.
const maxMacroExpandDepth = 256

// macroByName returns the macro FnDef bound to name, if any.
func macroByName(r *Registry, name string) (FnDefInfo, bool) {
	bound, ok := r.Defs.Top(name)
	if !ok {
		return FnDefInfo{}, false
	}
	fd, ok := bound.Data.(FnDefInfo)
	if !ok || !fd.Macro {
		return FnDefInfo{}, false
	}
	return fd, true
}

// expandAllMacros walks a token list expanding every macro call it finds — at
// the head of the list (`mac op…`, consuming the macro's arity in following
// tokens) and inside nested List / ParenExpr / Map values — recursing into
// each expansion. Non-macro tokens pass through. The depth bound guards
// runaway self-expansion.
func expandAllMacros(r *Registry, toks []Value, depth int) ([]Value, error) {
	if depth > maxMacroExpandDepth {
		return nil, &BoruError{Code: "macroexpand_error",
			Detail: "macroexpand: expansion too deep (recursive macro?)"}
	}
	out := make([]Value, 0, len(toks))
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if IsWord(t) {
			w, _ := AsWord(t)
			if fd, ok := macroByName(r, w.Name); ok {
				arity := len(fd.Signatures[0].Params)
				if i+1+arity <= len(toks) {
					operands := toks[i+1 : i+1+arity]
					exp, err := expandMacroWith(r, &fd, operands)
					if err != nil {
						return nil, err
					}
					rec, err := expandAllMacros(r, exp, depth+1)
					if err != nil {
						return nil, err
					}
					out = append(out, rec...)
					i += arity // operands consumed; loop's i++ skips the macro word
					continue
				}
				// Not enough following tokens for a full call — leave as-is.
			}
		}
		nested, err := expandAllNested(r, t, depth)
		if err != nil {
			return nil, err
		}
		out = append(out, nested)
	}
	return out, nil
}

// expandAllNested recurses expandAllMacros into a container value.
func expandAllNested(r *Registry, t Value, depth int) (Value, error) {
	switch {
	case t.Parent.Equal(TList) && IsConcrete(t):
		l, _ := AsList(t)
		inner, err := expandAllMacros(r, l.Slice(), depth+1)
		if err != nil {
			return Value{}, err
		}
		nl := NewList(inner)
		nl.Quoted = t.Quoted
		return nl, nil
	case IsParenExpr(t):
		pt, _ := AsParenExpr(t)
		inner, err := expandAllMacros(r, pt, depth+1)
		if err != nil {
			return Value{}, err
		}
		return NewParenExpr(inner), nil
	case t.Parent.Equal(TMap) && IsConcrete(t):
		m, _ := AsMap(t)
		if m == nil {
			return t, nil
		}
		out := NewOrderedMap()
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			rv, err := expandAllNested(r, v, depth)
			if err != nil {
				return Value{}, err
			}
			out.Set(k, rv)
		}
		return NewMap(out), nil
	default:
		return t, nil
	}
}
