package compiler

import core "github.com/boru-lang/boru/core/go"

// The kept-defs latch's PROOF side (kept_defs.go, NUR210): a computed body
// whose tokens the recorder can recover and prove binding-free keeps nothing,
// so the latch has no reason to arm. The common factory shape is exactly
// that — `def mk fn [[][List][quote [add k]]] end each (mk) [1] … each (mk)
// [1]` reads nothing the body could have changed.

// keptDefsAlwaysBinds are the words whose run changes a binding (or a type's
// behaviour) past any frame: `undef` pops the nearest binding wherever it
// lives, `behave` and `usurp` rewrite dispatch program-wide.
var keptDefsAlwaysBinds = map[string]bool{"undef": true, "behave": true, "usurp": true}

// keptDefsFrameBinders bind a name in the scope they run in — the enclosing
// scope for a kept-defs body, the frame's own inside a fn body.
var keptDefsFrameBinders = map[string]bool{"def": true, "var": true, "unpack": true}

// keptDefsScanDepth bounds the token walk; a body nested deeper is judged
// unproven, never proven.
const keptDefsScanDepth = 8

// bodyBindsNothing reports whether a computed body's tokens are KNOWN here
// and provably change no binding when run: the operand is a const list, or
// the single result of a user call whose unit returns one (the factory
// `(mk)` over `quote [add k]`), read directly or through the promoted
// value-def holding it (`def b (mk)`), and its tokens pass keptDefsScan.
func (es *EmitState) bodyBindsNothing(r *core.Registry, body core.Value) bool {
	toks, ok := es.provenBodyTokens(body)
	if !ok {
		return false
	}
	sc := &keptDefsScan{r: r, seen: map[string]bool{}}
	return sc.all(toks)
}

// provenBodyTokens is the token list a computed body operand holds at run
// time, when the recorder can prove it (see bodyBindsNothing).
func (es *EmitState) provenBodyTokens(body core.Value) ([]core.Value, bool) {
	op, ok := es.resolveOperand(body)
	if !ok {
		return nil, false
	}
	switch op.kind {
	case opEvent:
		if op.resIdx != 0 {
			return nil, false
		}
		op, ok = es.producerReturnedOutOpSeq(op.idx)
	case opLocal:
		pr, produced := es.producedBy[body.ID]
		if !produced || pr.idx != 0 {
			return nil, false
		}
		op, ok = es.producerReturnedOutOpSeq(pr.seq)
	}
	if !ok || op.kind != opConst || op.idx < 0 || op.idx >= len(es.consts) {
		return nil, false
	}
	lst, err := core.AsList(es.consts[op.idx])
	if err != nil || lst.IsNil() {
		return nil, false
	}
	return lst.Slice(), true
}

// keptDefsScan walks a body's tokens for anything that could change a
// binding when they run: a binder word, a word it cannot resolve, a reach or
// a splice (a dispatch it cannot see), a closure or an interpolation — and,
// through every word naming a user fn or a bound value, the fn's bodies and
// the value's contents (a bound list may be run as code: `do x`). Inside a
// fn body a frame binder binds the frame's own scope, nothing outside it; an
// always-binder never is. Each fn is visited once.
type keptDefsScan struct {
	r     *core.Registry
	seen  map[string]bool
	inFn  bool
	depth int
}

func (sc *keptDefsScan) value(v core.Value) bool {
	sc.depth++
	defer func() { sc.depth-- }()
	if sc.depth > keptDefsScanDepth {
		return false
	}
	if core.IsWord(v) {
		wi, _ := core.AsWord(v)
		return sc.word(wi.Name)
	}
	if core.IsReach(v) || core.IsSplice(v) {
		return false
	}
	if v.Carrier {
		// A computed binding: its run-time value is unknown, and one that
		// may hold code (a list, a map, a fn, or anything) may be run.
		t := v.Parent
		return !(v.Dynamic || t == nil || t.ConformsTo(core.TList) || t.ConformsTo(core.TMap) ||
			t.ConformsTo(core.TFunction) || core.TList.ConformsTo(t))
	}
	switch d := v.Data.(type) {
	case core.ListPayload:
		return sc.all(d.Elems)
	case core.ParenExprPayload:
		return sc.all(d.Toks)
	case core.MapPayload:
		if d.M == nil {
			return true
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if !sc.value(mv) {
				return false
			}
		}
	case core.FnDefInfo:
		return sc.fnBodies(d.Signatures)
	case core.ClosurePayload, core.InterpStringPayload, core.XmlInterpPayload:
		return false
	}
	return true
}

func (sc *keptDefsScan) all(vs []core.Value) bool {
	for _, v := range vs {
		if !sc.value(v) {
			return false
		}
	}
	return true
}

// word judges one word token: a binder, an unresolved name, or a word whose
// bound value or fn bodies do not pass, all fail.
func (sc *keptDefsScan) word(name string) bool {
	if keptDefsAlwaysBinds[name] || (!sc.inFn && keptDefsFrameBinders[name]) {
		return false
	}
	if bound, ok := sc.r.Defs.Top(name); ok {
		if _, isFn := bound.Data.(core.FnDefInfo); !isFn {
			return sc.value(bound)
		}
	}
	fn := sc.r.Lookup(name)
	if fn == nil {
		// Inside a fn body an unresolved name is the frame's own — a param,
		// a capture, a body-local def — and a read of one binds nothing: a
		// binder is a native, which always resolves, and a fn value a param
		// holds came from tokens this walk judges where they are written.
		// At the body's top level the name is unknown here, and unknown is
		// unproven.
		return sc.inFn || name == "true" || name == "false" || name == "none"
	}
	if sc.seen[name] {
		return true
	}
	sc.seen[name] = true
	for i := range fn.Signatures {
		if fn.Signatures[i].RunInCheckMode() && !(sc.inFn && keptDefsFrameBinders[name]) {
			return false
		}
	}
	return sc.fnBodies(fn.Signatures)
}

// fnBodies scans the user (non-native) bodies among sigs, each inside a frame
// of its own.
func (sc *keptDefsScan) fnBodies(sigs []core.Signature) bool {
	was := sc.inFn
	sc.inFn = true
	defer func() { sc.inFn = was }()
	for i := range sigs {
		if _, native := sigs[i].Impl.(*core.GoImpl); native {
			continue
		}
		if !sc.all(sigs[i].Body()) {
			return false
		}
	}
	return true
}

// bodyPlainData reports whether a computed body's tokens are proven
// (provenBodyTokens) and PLAIN DATA — no word, reach, splice, paren group,
// interpolation or fn value anywhere in them — so its run pushes exactly
// those values and none of them is callable: `quote [1 2]`.
func (es *EmitState) bodyPlainData(body core.Value) bool {
	_, ok := es.bodyPlainCount(body)
	return ok
}

// bodyPlainCount is bodyPlainData with the run's STATIC size: a plain-data
// body pushes each of its tokens as one value (a nested list or map literal
// included), so the run leaves exactly len(tokens) values. A computed `do`
// body proven to leave exactly one is no region at all (recordDynBodyCall).
func (es *EmitState) bodyPlainCount(body core.Value) (int, bool) {
	toks, ok := es.provenBodyTokens(body)
	if !ok {
		return 0, false
	}
	for _, tk := range toks {
		if valueRefsName(tk) || core.IsParenExpr(tk) || regionValsMayBeCallable([]core.Value{tk}) || engineMarkerToken(tk) {
			return 0, false
		}
	}
	return len(toks), true
}

// engineMarkerToken reports a token the engine steps as something other than
// one pushed value — a dispatch modifier, a sugar marker, a mark or a move.
func engineMarkerToken(tk core.Value) bool {
	_, mod := core.AsDispatchMod(tk)
	return mod || core.IsSugar(tk) || core.IsMark(tk) || core.IsMove(tk)
}
