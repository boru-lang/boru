package compiler

import core "github.com/boru-lang/boru/core/go"

// polyRawOperands is PolyRef.Raw for the poly call whose n operands top the
// walk's stack: each one the result of a list or map literal's assembly
// (OpMakeList / OpMakeMap) the event lists being lowered hold, whose literal
// is a pending token of the body — `each (h) [1 add 2]`, the collection
// evaluated before each is matched (NUR352, NUR356). Nil when none is.
func (lw *lowerer) polyRawOperands(n int) map[int]core.Value {
	var raw map[int]core.Value
	for i := 0; i < n && i < len(lw.vm); i++ {
		s := lw.vm[len(lw.vm)-1-i]
		ev := lw.scopeEvent(s.seq)
		if s.idx != 0 || ev == nil || !rawRenderedAssembly(ev) {
			continue
		}
		if tok, ok := literalTokenAt(lw.sourceBody(), ev.call.pos); ok {
			if raw == nil {
				raw = map[int]core.Value{}
			}
			raw[i] = tok
		}
	}
	return raw
}

// sourceBody is the body the events being lowered were recorded from: the
// program's, or the fn unit's.
func (lw *lowerer) sourceBody() []core.Value {
	if lw.landingBody == nil && lw.rec != nil {
		return lw.rec.body
	}
	return lw.landingBody
}

// scopeEvent is the event seq among the event lists being lowered
// (lowerer.scopes), or nil.
func (lw *lowerer) scopeEvent(seq int) *EmitEvent {
	for _, events := range lw.scopes {
		for i := range events {
			if events[i].seq == seq {
				return &events[i]
			}
		}
	}
	return nil
}

// pendingListAt is the pending list literal token of body written at p — at
// any depth of the body's parens and list literals — or false when none is.
func pendingListAt(body []core.Value, p core.SrcPos) (core.Value, bool) {
	t, ok := literalTokenAt(body, p)
	return t, ok && t.Parent.Equal(core.TList)
}

// literalTokenAt is the pending list or map literal token of body written
// at p — at any depth of the body's parens and list literals — or false.
func literalTokenAt(body []core.Value, p core.SrcPos) (core.Value, bool) {
	if p.Row == 0 {
		return core.Value{}, false
	}
	toks := body
	path := tokenPath(body, p)
	for k, at := range path {
		t := toks[at]
		if k == len(path)-1 {
			return t, t.Pos() == p && t.Eval && !t.Quoted && (t.Parent.Equal(core.TList) || t.Parent.Equal(core.TMap))
		}
		toks, _ = nestedToks(t)
	}
	return core.Value{}, false
}

// rawRenderedAssembly reports whether ev is a list or map literal's assembly
// (OpMakeList / OpMakeMap) — an operand a poly's no-match report renders as
// the literal written (polyRawOperands).
func rawRenderedAssembly(ev *EmitEvent) bool {
	return ev.kind == evCall && (ev.call.makeList || ev.call.makeMap)
}
