package core

// DispatchLayout is where a compiling pass's dispatch found its operands on
// the tape, published for the recorder while the dispatch is recorded
// (CheckState.CurLayout) — NUR242's `fold` over a gradual class field.
//
// A poly record re-matches at run time, and when nothing matches it has
// to raise the interpreter's signature_error, which the interpreter builds
// from its TAPE at the word: the collection plan over the stack beneath
// and the operands written after, then the attempted window over the same
// tape. The poly op holds only the operands, in signature order, so it
// cannot tell a written operand from a stack one, and for a word with
// overloads of more than one arity (fold's 2- and 3-operand forms) the
// split decides what the interpreter's plan claims. The layout says it.
//
// It is published only when the interpreter's tape at a failed dispatch can
// be REBUILT: the operands are the tape's own values, contiguous on each
// side of the word (the written ones after it in signature order, the stack
// ones beneath it top first), and what else the plan could reach up to the
// statement or group boundary (or the tape's end) on each side rides with
// it — constant values beneath the operands (Beneath) and source tokens
// after them (After, layoutSurround). The tape is then exactly [Beneath,
// stack operands, word, written operands, After], so a rebuild from the
// operands is its plan and its report, byte for byte. Any other shape
// publishes nothing, and the record keeps the defer it had. With nothing
// beneath or after, the layout is EXACT (NUR242); the rest is NUR283's.
type DispatchLayout struct {
	// args is the operand slice the layout describes; a record reads the
	// layout only for this slice (LayoutFor).
	args []Value
	// NFwd is how many leading signature positions were written after the
	// word; the rest came off the stack beneath it, top first.
	NFwd int
	// Beneath are the values between the boundary below and the stack
	// operands, in tape order — constants, which the run's tape holds as
	// the pass's does.
	Beneath []Value
	// After are the source tokens between the written operands and the
	// boundary after them, in tape order: scalar literals, closed by a
	// function word that bars the forward collection.
	After []Value
	// Words, by written operand index, is the word token a written operand
	// was read from where the tape still holds it (exactLayout): the
	// interpreter's plan reads a data binding without stepping its word, so
	// its report stops there (NUR356).
	Words map[int]Value
	// Live, on an optimistic dispatch's layout, indexes the Beneath entries
	// that are no constant — a call's result, a carrier — whose value the
	// run's stack holds there, not the pass's tape (NUR351). A record that
	// cannot say where the compiled code keeps each one reads no layout.
	Live []int
}

// writtenPub is CheckState.CurWritten: how many leading operands (signature
// positions 0..nFwd-1) of one dispatch were written after its word, for the
// dispatch's own operand slice (NUR362). Unlike a DispatchLayout it claims
// nothing about the tape around the operands, so it is published wherever
// the split is known.
type writtenPub struct {
	args []Value
	nFwd int
}

// WrittenFor returns the published written count when it describes args —
// the very slice the dispatch published it for, as LayoutFor answers — and
// false for any other record.
func (c *CheckState) WrittenFor(args []Value) (int, bool) {
	p := c.CurWritten
	if p == nil || len(args) == 0 || len(p.args) != len(args) || &p.args[0] != &args[0] {
		return 0, false
	}
	return p.nFwd, true
}

// PublishWritten publishes that the leading nFwd of args (signature order)
// were written after the dispatching word, and returns the restore the
// caller runs once the dispatch is recorded. Only a recording pass
// publishes, and only a split that holds a written operand.
func (e *Engine) PublishWritten(args []Value, nFwd int) func() {
	if nFwd <= 0 || nFwd > len(args) || !e.Registry.analysisRecorder().Active() {
		return func() {}
	}
	prev := e.Registry.Check.CurWritten
	e.Registry.Check.CurWritten = &writtenPub{args: args, nFwd: nFwd}
	return func() { e.Registry.Check.CurWritten = prev }
}

// BarrierBars reports whether an overload among sigs stops its forward
// collection short of nFwd written operands and then reads the stack: its
// barrier (the unmodified word's forward limit) is below nFwd and below its
// own arity. The interpreter's plan for such an overload takes the leading
// operands up to its barrier and the rest off the stack beneath the word —
// never the window a dispatch whose leading nFwd operands were written
// after the word holds — so a flat match of that window against it is not
// the interpreter's match (NUR362: `get (h) {k:1}`, every get overload's
// barrier 1, raises signature_error however (h) comes out).
func BarrierBars(sigs []Signature, nFwd int) bool {
	for i := range sigs {
		if s := &sigs[i]; !s.Fallback && s.BarrierPos >= 0 && s.BarrierPos < nFwd && s.BarrierPos < s.TotalArgs() {
			return true
		}
	}
	return false
}

// LayoutFor returns the published layout when it describes args — the
// very slice the dispatch published it for, which the record path hands
// down unchanged — and nil for any other record, which a nested analysis
// may make while the layout is published.
func (c *CheckState) LayoutFor(args []Value) *DispatchLayout {
	l := c.CurLayout
	if l == nil || len(args) == 0 || len(l.args) != len(args) || &l.args[0] != &args[0] {
		return nil
	}
	return l
}

// PublishLayout publishes the exact layout of the dispatch at the pointer
// whose operands args (signature order) sit at the tape indices at, and
// returns the restore the caller runs once the dispatch is recorded. A
// layout that is not exact publishes nil, so no record reads a stale one.
// Only a recording pass publishes.
func (e *Engine) PublishLayout(args []Value, at []int, pos SrcPos) func() {
	prev := e.Registry.Check.CurLayout
	e.Registry.Check.CurLayout = e.exactLayout(args, at, pos)
	return func() { e.Registry.Check.CurLayout = prev }
}

// exactLayout is the layout when it is exact (see DispatchLayout), else nil.
func (e *Engine) exactLayout(args []Value, at []int, pos SrcPos) *DispatchLayout {
	n := len(args)
	if n == 0 || len(at) != n || !e.Registry.analysisRecorder().Active() {
		return nil
	}
	w, err := AsWord(e.Tape.At(e.Pointer))
	if err != nil || !unmodifiedWord(w) {
		return nil
	}
	// The report's two tape-only layers must not apply (the probe's rule,
	// PolyNoMatchProbe): a void argument group, the fn-shape binding hint.
	if e.voidArgErrorFor(w.Name, pos) != nil || e.IsFnShapeTypedBindingContext() {
		return nil
	}
	k := 0
	for k < n && at[k] > e.Pointer {
		k++
	}
	var words map[int]Value
	for i := 0; i < n; i++ {
		want := e.Pointer + 1 + i
		if i >= k {
			want = e.Pointer - (i - k) - 1
		}
		if at[i] != want || want < 0 || want >= e.Tape.Len() || args[i].ID == "" {
			return nil
		}
		if tok := e.Tape.At(want); tok.ID != args[i].ID {
			// A written word whose data binding is the operand: the plan
			// read it and left the word on the tape (Words).
			if i >= k || !e.readsDataBinding(tok, args[i]) {
				return nil
			}
			if words == nil {
				words = map[int]Value{}
			}
			words[i] = tok
		}
	}
	beneath, after, _, ok := e.layoutSurround(e.Pointer-(n-k)-1, e.Pointer+k+1, k, false)
	if !ok {
		return nil
	}
	return &DispatchLayout{args: args, NFwd: k, Beneath: beneath, After: after, Words: words}
}

// readsDataBinding reports whether tok is a bare word whose binding is the
// operand v, and data (readsData) of a type the pass knows: the
// interpreter's plan reads such a word's binding where it stands and steps
// nothing (a fn binding is a collection barrier instead, and a gradual one
// may be either).
func (e *Engine) readsDataBinding(tok, v Value) bool {
	w, err := AsWord(tok)
	if err != nil || w.ForceVal {
		return false
	}
	top, ok := e.Registry.Defs.Top(w.Name)
	return ok && top.ID == v.ID && !top.Dynamic && readsData(e.Registry, w.Name)
}

// layoutSurround reads what a dispatch's plan could reach around its
// operands, from the tape index below them down to the statement or group
// boundary beneath (an open paren, an `end`, the tape's start) and from the
// index after them up to the boundary above (a close paren, an `end`, the
// tape's end). ok is false unless every value beneath is a constant the
// run's tape holds as the pass's does (layoutConstant) — or, where live
// allows it, a value the run's stack holds (liveBeneath, listed in liveIdx
// by its Beneath index) — and every token after is a scalar literal, the
// run closed by a bare function word — the next dispatch, which bars the
// forward collection on both lanes (NUR283) — or by any word standing past
// every forward position the word's overloads reach (forwardReachMax, nFwd
// of them already written): no plan steps it, and the report's walk stops
// at it (NUR356).
func (e *Engine) layoutSurround(below, after, nFwd int, live bool) (beneath, afterToks []Value, liveIdx []int, ok bool) {
	var fromTop []bool
	for i := below; i >= 0 && !IsOpenParen(e.Tape.At(i)) && !IsEnd(e.Tape.At(i)); i-- {
		v := e.Tape.At(i)
		isLive := false
		if !layoutConstant(v, true) {
			if !live || !liveBeneath(v) {
				return nil, nil, nil, false
			}
			isLive = true
		}
		beneath = append([]Value{v}, beneath...)
		fromTop = append(fromTop, isLive)
	}
	for j, isLive := range fromTop {
		if isLive {
			liveIdx = append(liveIdx, len(fromTop)-1-j)
		}
	}
	for i := after; i < e.Tape.Len() && !IsCloseParen(e.Tape.At(i)) && !IsEnd(e.Tape.At(i)); i++ {
		tok := e.Tape.At(i)
		if layoutConstant(tok, false) {
			afterToks = append(afterToks, tok)
			continue
		}
		if w, err := AsWord(tok); err == nil && !tok.Quoted && unmodifiedWord(w) && FnWordBarrierOn(e.Registry, tok) {
			return beneath, append(afterToks, tok), liveIdx, true
		}
		if reach, known := e.forwardReachMax(); IsWord(tok) && !tok.Quoted && known && nFwd+len(afterToks) >= reach {
			return beneath, append(afterToks, tok), liveIdx, true
		}
		return nil, nil, nil, false
	}
	return beneath, afterToks, liveIdx, true
}

// forwardReachMax is the most forward positions any overload of the word at
// the pointer can claim (effectiveForwardLimit, an all-forward barrier its
// arity), and false when the pointer holds no word the registry names.
func (e *Engine) forwardReachMax() (int, bool) {
	w, err := AsWord(e.Tape.At(e.Pointer))
	if err != nil {
		return 0, false
	}
	fn := e.Registry.Lookup(w.Name)
	if fn == nil {
		return 0, false
	}
	most := 0
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		limit := effectiveForwardLimit(s, w)
		if limit < 0 || limit > s.TotalArgs() {
			limit = s.TotalArgs()
		}
		most = max(most, limit)
	}
	return most, true
}

// liveBeneath reports whether v, a value beneath a dispatch's operands that
// is no layout constant, is one the run's stack holds as a value of its own
// — a call's result, a carrier — which a record can place by its identity:
// never a word, a marker, or a value with no ID.
func liveBeneath(v Value) bool {
	return v.ID != "" && !IsWord(v) && !isEngineMarker(v) && !IsForward(v) && !IsParenExpr(v) &&
		(v.Carrier || v.Dynamic || IsConcrete(v))
}

// unmodifiedWord reports whether w carries no modifier the source wrote —
// no `/N` count, no forced stack, forward, value or usurp read.
func unmodifiedWord(w WordInfo) bool {
	return w.ArgCount == -1 && !w.ForceStack && !w.ForceForward && !w.ForceVal && !w.ForceUsurp
}

// layoutConstant reports whether v is a value a rebuilt tape can hold as
// the pass's tape holds it: a concrete scalar — a number, a string, a
// boolean, an atom — or, beneath the operands (where values are already
// resolved), a concrete list.
func layoutConstant(v Value, lists bool) bool {
	if !IsConcrete(v) || v.Carrier || v.Dynamic || v.Quoted || v.Parent == nil {
		return false
	}
	p := v.Parent
	return p.ConformsTo(TNumber) || p.ConformsTo(TString) || p.ConformsTo(TBoolean) || p.ConformsTo(TAtom) ||
		(lists && p.ConformsTo(TList))
}

// SigOrderPositions is SigOrderArgs over tape indices: positions lists the
// stack run (nStack of them, ascending toward the pointer) and then the
// forward run in source order; the result lists them in signature order.
func SigOrderPositions(positions []int, nStack int) []int {
	if nStack < 0 || nStack > len(positions) {
		nStack = len(positions)
	}
	out := make([]int, 0, len(positions))
	out = append(out, positions[nStack:]...)
	for i := nStack - 1; i >= 0; i-- {
		out = append(out, positions[i])
	}
	return out
}
