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

// writtenPositions is where each of args' first nFwd operands — the ones
// written after the word — was read, for splitWords: the position of the
// latest read of its binding (NoteLocalRead, which the forward collection's
// step of the word just made), else its own.
func (es *EmitState) writtenPositions(args []core.Value, nFwd int) []core.SrcPos {
	out := make([]core.SrcPos, 0, nFwd)
	for i := 0; i < nFwd && i < len(args); i++ {
		p := args[i].Pos()
		if rp, ok := es.readPos[args[i].ID]; ok && args[i].ID != "" {
			p = rp
		}
		out = append(out, p)
	}
	return out
}

// splitWords is sp with its Words (PolySplit.Words, NUR356) completed: each
// written operand whose read position (fwdPos) is a word token standing
// after the dispatching word (at wordPos) in the same token sequence of the
// body being lowered — a read the interpreter's plan leaves on its tape,
// where its no-match report stops. The pass's forward collection stepped
// such a word before an optimistic match (the layout's own Words cover a
// failed dispatch's tape, which still holds it). sp itself when none is
// added.
func (lw *lowerer) splitWords(sp *PolySplit, wordPos core.SrcPos, fwdPos []core.SrcPos) *PolySplit {
	if sp == nil || len(fwdPos) == 0 {
		return sp
	}
	toks, at, ok := siblingTokens(lw.sourceBody(), wordPos)
	if !ok {
		return sp
	}
	var words map[int]core.Value
	for i, p := range fwdPos {
		j := bodyTokenAt(toks, p)
		if _, known := sp.Words[i]; known || j <= at || !core.IsWord(toks[j]) {
			continue
		}
		if words == nil {
			words = make(map[int]core.Value, len(sp.Words)+1)
			for k, w := range sp.Words {
				words[k] = w
			}
		}
		words[i] = toks[j]
	}
	if words == nil {
		return sp
	}
	out := *sp
	out.Words = words
	return &out
}

// siblingTokens is the token sequence of body — the body itself, or a
// paren's or list literal's at any depth — holding the token written at p,
// and its index there; false when no token stands exactly at p.
func siblingTokens(body []core.Value, p core.SrcPos) ([]core.Value, int, bool) {
	path := tokenPath(body, p)
	if len(path) == 0 {
		return nil, 0, false
	}
	toks := body
	for _, at := range path[:len(path)-1] {
		toks, _ = nestedToks(toks[at])
	}
	at := path[len(path)-1]
	return toks, at, toks[at].Pos() == p
}
