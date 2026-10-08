package core

import (
	"strings"
	"testing"
)

// Core-suite pins for NUR358 (a container literal is not a loop), NUR235 (a
// pattern judges a pending literal's value) and the recovery trap's raw
// rendering — driven over hand-built tapes and small native words, the way
// the rest of core's suite drives the engine.

// nurReg is covRegistry plus the words these tests step: nbrk (break),
// ncont (continue), nidl / nidm (take a List / Map operand and hand it
// back), and npm / npl (a Map / List operand under a contents pattern, each
// with a second patterned overload).
func nurReg(t *testing.T) *Registry {
	t.Helper()
	intPat := NewOrderedMap()
	intPat.Set("f", NewTypeLiteral(TInteger))
	strPat := NewOrderedMap()
	strPat.Set("f", NewTypeLiteral(TString))
	answer := func(s string) *GoImpl {
		return Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewString(s)}, nil
		})
	}
	same := Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{args[0]}, nil
	})
	flow := func(f FlowCtrl) *GoImpl {
		return Go(func(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
			r.FlowCtrl = f
			return nil, nil
		})
	}
	return covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{Name: "nbrk", Signatures: []Signature{{Impl: flow(FlowBreak), Returns: []*Type{}, BarrierPos: -1}}})
		r.RegisterNativeFunc(NativeFunc{Name: "ncont", Signatures: []Signature{{Impl: flow(FlowContinue), Returns: []*Type{}, BarrierPos: -1}}})
		r.RegisterNativeFunc(NativeFunc{Name: "nidl", Signatures: []Signature{{Args: []*Type{TList}, Impl: same, Returns: []*Type{TList}, BarrierPos: 0}}})
		r.RegisterNativeFunc(NativeFunc{Name: "nidm", Signatures: []Signature{{Args: []*Type{TMap}, Impl: same, Returns: []*Type{TMap}, BarrierPos: 0}}})
		r.RegisterNativeFunc(NativeFunc{Name: "npm", Signatures: []Signature{
			{Args: []*Type{TMap}, Patterns: map[int]Value{0: NewMap(intPat)}, Impl: answer("int"), Returns: []*Type{TString}, BarrierPos: 0},
			{Args: []*Type{TMap}, Patterns: map[int]Value{0: NewMap(strPat)}, Impl: answer("str"), Returns: []*Type{TString}, BarrierPos: 0},
		}})
		r.RegisterNativeFunc(NativeFunc{Name: "npl", Signatures: []Signature{
			{Args: []*Type{TList}, Patterns: map[int]Value{0: NewList([]Value{NewInteger(2)})}, Impl: answer("two"), Returns: []*Type{TString}, BarrierPos: 0},
			{Args: []*Type{TList}, Patterns: map[int]Value{0: NewList([]Value{NewInteger(3)})}, Impl: answer("three"), Returns: []*Type{TString}, BarrierPos: 0},
		}})
	})
}

// group is a paren expression over toks — an active token in a literal.
func group(toks ...Value) Value { return NewParenExpr(toks) }

// evalMap is a pending map literal of the given key/value pairs.
func evalMap(kv ...Value) Value {
	m := NewOrderedMap()
	for i := 0; i+1 < len(kv); i += 2 {
		k, _ := AsString(kv[i])
		m.Set(k, kv[i+1])
	}
	return NewEvalMap(m)
}

// wantFlowError runs prog and requires `<ctrl> outside loop`.
func wantFlowError(t *testing.T, r *Registry, prog []Value, ctrl string) {
	t.Helper()
	_, err := NewTop(r).Run(prog)
	be, ok := err.(*BoruError)
	if !ok || be.Code != "flow_error" || !strings.Contains(be.Detail, ctrl+" outside loop") {
		t.Fatalf("Run = %v, want flow_error %s outside loop", err, ctrl)
	}
	if r.FlowCtrl != FlowNone || r.FlowAtHeld {
		t.Fatalf("the raise must take the signal: flow=%v held=%v", r.FlowCtrl, r.FlowAtHeld)
	}
}

// TestNUR358CoreLiteralEscapeRaises: a signal escaping a literal's elements
// with no loop on the run raises at the top — the residual sweep, a nested
// literal (the inner run's position held), a consumed List / Map operand,
// a map member before others and the only one, and a frame's residual.
func TestNUR358CoreLiteralEscapeRaises(t *testing.T) {
	brk := NewWord("nbrk")
	for name, prog := range map[string][]Value{
		"residual list":   {NewEvalList([]Value{brk})},
		"nested list":     {NewEvalList([]Value{NewEvalList([]Value{NewInteger(1), brk, NewInteger(2)})})},
		"consumed list":   {NewEvalList([]Value{brk}), NewWord("nidl")},
		"consumed map":    {evalMap(NewString("a"), group(brk)), NewWord("nidm")},
		"member first":    {evalMap(NewString("a"), group(brk), NewString("b"), NewInteger(2))},
		"member only":     {evalMap(NewString("a"), group(brk))},
		"frame residual":  {NewEvalList([]Value{brk}), NewDefCleanup(DefCleanupInfo{SkipCleanup: true, EvalResidual: true})},
		"continue escape": {NewEvalList([]Value{NewWord("ncont")})},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := "break"
			if name == "continue escape" {
				ctrl = "continue"
			}
			wantFlowError(t, nurReg(t), prog, ctrl)
		})
	}
	// The held position is the inner run's: a token after the signal
	// inside the literal carries it.
	r := nurReg(t)
	after := WithPos(NewInteger(2), Value{pos: &SrcPos{Row: 3, Col: 7}})
	_, err := NewTop(r).Run([]Value{NewEvalList([]Value{brk, after})})
	if be, ok := err.(*BoruError); !ok || be.Row != 3 || be.Col != 7 {
		t.Fatalf("Run = %v, want the escaped run's position 3:7", err)
	}
	// Negative: a literal that raises nothing is evaluated as ever.
	out, err := NewTop(nurReg(t)).Run([]Value{NewEvalList([]Value{NewInteger(1), group(NewInteger(2))})})
	if err != nil || renderAll(out) != "[1 2]" {
		t.Fatalf("Run = %v / %v, want [1 2]", renderAll(out), err)
	}
}

// TestNUR358CoreLoopTakesTheEscape: inside a loop the literal's escape is
// the loop's — the iteration's collection is undone and the loop breaks.
func TestNUR358CoreLoopTakesTheEscape(t *testing.T) {
	r := nurReg(t)
	cont := &ForCont{Registry: r, IterName: "nuri", Current: 0, End: 3, Step: 1, Results: []Value{NewInteger(42)},
		Body: []Value{NewInteger(9), NewEvalList([]Value{NewWord("nbrk")})}}
	InstallDef(r, "nuri", NewInteger(0))
	prog := []Value{
		NewMark("nurL"),
		NewInteger(9), NewEvalList([]Value{NewWord("nbrk")}),
		NewMoveCont("nurL", "for loop", cont),
	}
	out, err := NewTop(r).Run(prog)
	if err != nil || renderAll(out) != "42" {
		t.Fatalf("Run = %s / %v, want 42 (the iteration's 9 dropped)", renderAll(out), err)
	}
}

// TestNUR358CoreSubEngineContracts: a plain pooled sub-run hands its
// unstepped tape back (the outer run resolves the signal), a container
// sub-run tears down and returns nothing, and an inner run's held position
// survives an enclosing one's.
func TestNUR358CoreSubEngineContracts(t *testing.T) {
	r := nurReg(t)
	res, err := RunPooledSub(r, []Value{NewWord("nbrk"), NewInteger(5)}, false)
	if err != nil || len(res) == 0 || r.FlowCtrl != FlowBreak {
		t.Fatalf("pooled sub-run = %v / %v (flow %v), want its residual and the signal", res, err, r.FlowCtrl)
	}
	r.TakeFlow()
	res, err = RunContainerSub(r, []Value{NewWord("nbrk"), NewInteger(5)}, false)
	if err != nil || len(res) != 0 || r.FlowCtrl != FlowBreak || !r.FlowAtHeld {
		t.Fatalf("container sub-run = %v / %v (flow %v held %v), want nothing and a held signal", res, err, r.FlowCtrl, r.FlowAtHeld)
	}
	first := r.FlowAt
	r.HoldFlowAt(SrcPos{Row: 99, Col: 1}, true)
	if r.FlowAt != first {
		t.Fatalf("an inner run's held position must survive: %v, want %v", r.FlowAt, first)
	}
	r.TakeFlow()
}

// TestNUR235CorePatternEvaluatesThePendingLiteral: a pending literal at a
// contents-pattern slot is matched as its value (map and list patterns), an
// evaluation error abandons the dispatch without a second evaluation, and
// an escape abandons it for the run to resolve.
func TestNUR235CorePatternEvaluatesThePendingLiteral(t *testing.T) {
	two := group(NewInteger(2))
	for _, c := range []struct {
		prog []Value
		want string
	}{
		{[]Value{evalMap(NewString("f"), two), NewWord("npm")}, "'int'"},
		{[]Value{evalMap(NewString("f"), group(NewString("s"))), NewWord("npm")}, "'str'"},
		{[]Value{NewEvalList([]Value{two}), NewWord("npl")}, "'two'"},
		{[]Value{NewEvalList([]Value{group(NewInteger(3))}), NewWord("npl")}, "'three'"},
	} {
		out, err := NewTop(nurReg(t)).Run(c.prog)
		if err != nil || renderAll(out) != c.want {
			t.Errorf("%v: got %s / %v, want %s", c.prog, renderAll(out), err, c.want)
		}
	}
	// An evaluation error at the first patterned candidate is the call's
	// error; the second candidate does not evaluate the literal again.
	for _, prog := range [][]Value{
		{evalMap(NewString("f"), group(NewWord("nur-missing"))), NewWord("npm")},
		{NewEvalList([]Value{group(NewWord("nur-missing"))}), NewWord("npl")},
	} {
		_, err := NewTop(nurReg(t)).Run(prog)
		if err == nil || !strings.Contains(err.Error(), "nur-missing") {
			t.Errorf("%v: got %v, want the member's undefined word", prog, err)
		}
	}
	// An escape abandons the dispatch; the run raises it.
	wantFlowError(t, nurReg(t), []Value{evalMap(NewString("f"), group(NewWord("nbrk"))), NewWord("npm")}, "break")
	// A fn VALUE at the pointer takes the same match.
	r := nurReg(t)
	npm, _ := r.Defs.Top("npm")
	fd, _ := npm.Data.(FnDefInfo)
	fd.Anonymous = true
	_, err := NewTop(r).Run([]Value{evalMap(NewString("f"), group(NewWord("nur-missing"))), NewFunction(fd)})
	if err == nil || !strings.Contains(err.Error(), "nur-missing") {
		t.Errorf("fn value dispatch: got %v, want the member's undefined word", err)
	}
	// The explain pass cannot blame a slot for a literal it never judged.
	intPat := NewOrderedMap()
	intPat.Set("f", NewTypeLiteral(TInteger))
	if patternRejects(NewMap(intPat), evalMap(NewString("f"), two)) {
		t.Error("an unevaluated literal must not be blamed by the probe")
	}
}

// TestNUR358CoreFnValueOperandEscape: a handler-less fn value stack-matched
// (execFnDefSig) abandons the call when its List or Map operand's literal
// escapes.
func TestNUR358CoreFnValueOperandEscape(t *testing.T) {
	for name, c := range map[string]struct {
		typ *Type
		arg Value
	}{
		"list": {TList, NewEvalList([]Value{NewInteger(1), group(NewWord("nbrk"))})},
		"map":  {TMap, evalMap(NewString("a"), group(NewWord("nbrk")))},
	} {
		t.Run(name, func(t *testing.T) {
			fv := NewFunction(FnDefInfo{Name: "nurg", Signatures: []Signature{{
				Params: []FnParam{{Name: "xs", Type: c.typ}},
				Impl:   Boru([]Value{NewWord("xs")}),
			}}})
			wantFlowError(t, nurReg(t), []Value{c.arg, fv}, "break")
		})
	}
}

// TestNUR235CoreRecoveryRawTrap: a literal a check-mode recovery evaluated
// in place renders as written in the definite trap's report (the tape is
// restored after), and a rematch over such a window declines.
func TestNUR235CoreRecoveryRawTrap(t *testing.T) {
	raw := evalMap(NewString("f"), group(NewInteger(2)))
	folded := NewOrderedMap()
	folded.Set("f", NewInteger(2))
	e, es := trapEngine(t, []Value{NewWord("hd"), NewMap(folded), NewInteger(5)}, 0, []int{1, 2})
	e.NoteRecoveryRaw(1, raw)
	fn := &FnDefInfo{Name: "trapw", Signatures: []Signature{{Fallback: true}, {Args: []*Type{TString, TInteger}, BarrierPos: 2}}}
	if !e.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "trapw"}, fn, SrcPos{Row: 1}) {
		t.Fatal("the definite failure must record the trap")
	}
	if len(es.trapErrs) != 1 || !strings.Contains(es.trapErrs[0].Error(), "{f:(2)}") {
		t.Errorf("trap = %v, want the literal as written", es.trapErrs)
	}
	if got := e.Tape.At(1); IsPendingActiveContainer(got) {
		t.Error("the evaluated map must be restored after the report")
	}
	e.ClearRecoveryRaw()
	if e.recoveryRaw != nil {
		t.Error("ClearRecoveryRaw must forget the literals")
	}
	// A carrier in the window routes to the rematch, which declines over
	// a recovered literal.
	e2, es2 := trapEngine(t, []Value{NewWord("hd"), NewMap(folded), NewCarrier(TInteger)}, 0, []int{1, 2})
	e2.NoteRecoveryRaw(1, raw)
	if e2.TryRecordUnmatchedDispatchTrap(WordInfo{Name: "trapw"}, fn, SrcPos{Row: 1}) || es2.rematches != 0 {
		t.Errorf("a rematch over a recovered literal must decline (rematches %d)", es2.rematches)
	}
}

// TestNUR336CoreNoteParenStack: the paren-stack note is taken over a tape
// of values alone and skipped over an open paren beneath the group.
func TestNUR336CoreNoteParenStack(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(1), NewInteger(2), NewOpenParen()}, StackHeadroom)
	e.noteParenStack(2)
	e.Tape = NewTape([]Value{NewOpenParen(), NewInteger(2), NewOpenParen()}, StackHeadroom)
	e.noteParenStack(2)
}

// TestNUR365CoreParenOperandEscape: a break escaping a paren a word
// collects forward abandons the word — at the top the run raises where the
// group's run stood, in a loop the loop breaks — and a frame the group
// spliced is torn down with it (the caller's args list is the one left).
func TestNUR365CoreParenOperandEscape(t *testing.T) {
	fwd := func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{Name: "nfw", Signatures: []Signature{{
			Args: []*Type{TInteger},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0]}, nil
			}),
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}}})
	}
	brk := NewWord("nbrk")
	after := WithPos(NewInteger(2), Value{pos: &SrcPos{Row: 4, Col: 9}})
	r := nurReg(t)
	fwd(r)
	_, err := NewTop(r).Run([]Value{NewWord("nfw"), NewOpenParen(), NewInteger(1), brk, after, NewCloseParen()})
	if be, ok := err.(*BoruError); !ok || be.Code != "flow_error" || be.Row != 4 || be.Col != 9 {
		t.Fatalf("Run = %v, want flow_error at the group's run (4:9)", err)
	}
	// In a loop: the word is abandoned and the loop breaks, the
	// iteration's partial values dropped.
	r = nurReg(t)
	fwd(r)
	cont := &ForCont{Registry: r, IterName: "nuri", Current: 0, End: 3, Step: 1, Results: []Value{NewInteger(42)}}
	InstallDef(r, "nuri", NewInteger(0))
	out, err := NewTop(r).Run([]Value{
		NewMark("nurP"), NewInteger(9),
		NewWord("nfw"), NewOpenParen(), NewInteger(1), brk, NewCloseParen(),
		NewMoveCont("nurP", "for loop", cont),
	})
	if err != nil || renderAll(out) != "42" {
		t.Fatalf("Run = %s / %v, want 42", renderAll(out), err)
	}
	// A fn frame spliced inside the group is torn down: its args entry
	// does not outlive the abandoned call.
	r = nurReg(t)
	fwd(r)
	InstallFnDef(r, "nurf", FnDefInfo{Signatures: []Signature{{
		Params: []FnParam{{Name: "x", Type: TInteger}}, Returns: []*Type{TAny},
		Impl: Boru([]Value{brk}), BarrierPos: BarrierAllForward,
	}}})
	depth := r.Args.Depth()
	cont = &ForCont{Registry: r, IterName: "nuri", Current: 0, End: 3, Step: 1}
	if _, err := NewTop(r).Run([]Value{
		NewMark("nurQ"),
		NewWord("nfw"), NewOpenParen(), NewWord("nurf"), NewInteger(1), NewCloseParen(),
		NewMoveCont("nurQ", "for loop", cont),
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.Args.Depth() != depth {
		t.Fatalf("args depth %d after the loop, want %d (the callee's frame unwound)", r.Args.Depth(), depth)
	}
	// Negative: a quiet group is the word's argument as ever.
	r = nurReg(t)
	fwd(r)
	out, err = NewTop(r).Run([]Value{NewWord("nfw"), NewOpenParen(), NewInteger(5), NewCloseParen()})
	if err != nil || renderAll(out) != "5" {
		t.Fatalf("Run = %s / %v, want 5", renderAll(out), err)
	}
	// A loop the group itself holds takes the signal first: the word
	// collects the loop's results — from inside a paren nested in the loop
	// too, whose open marker leaves with the loop region.
	for _, body := range [][]Value{{brk}, {NewOpenParen(), brk, NewCloseParen()}} {
		r = nurReg(t)
		fwd(r)
		inner := &ForCont{Registry: r, IterName: "nuri", Current: 0, End: 3, Step: 1, Results: []Value{NewInteger(7)}}
		InstallDef(r, "nuri", NewInteger(0))
		toks := append([]Value{NewWord("nfw"), NewOpenParen(), NewMark("nurQ")}, body...)
		toks = append(toks, NewMoveCont("nurQ", "for loop", inner), NewCloseParen())
		out, err = NewTop(r).Run(toks)
		if err != nil || renderAll(out) != "7" || r.FlowCtrl != FlowNone {
			t.Fatalf("Run = %s / %v (flow %v), want 7 with the signal taken", renderAll(out), err, r.FlowCtrl)
		}
	}
	// The depth read back counts a closed group's markers out.
	if d := openDepthBetween(NewTape([]Value{NewOpenParen(), NewOpenParen(), NewCloseParen(), NewInteger(1)}, StackHeadroom), 0, 4); d != 1 {
		t.Fatalf("openDepthBetween = %d, want 1", d)
	}
	// groupExtent over a group that never closes runs to the tape's end.
	if n := groupExtent(NewTape([]Value{NewOpenParen(), NewInteger(1)}, StackHeadroom), 0); n != 2 {
		t.Fatalf("groupExtent of an open group = %d, want 2", n)
	}
}

// TestNUR358CoreCallBoruDeferredResidualEscape: an anonymous lambda whose
// body is a single literal defers its residual past the frame; a signal
// escaping that literal raises `outside loop` inside the call, as one the
// body's own tokens raised does.
func TestNUR358CoreCallBoruDeferredResidualEscape(t *testing.T) {
	r := nurReg(t)
	sig := &FnSig{Anonymous: true, Impl: Boru([]Value{NewEvalList([]Value{NewWord("nbrk")})})}
	_, err := r.CallBoru(sig, nil, nil)
	be, ok := err.(*BoruError)
	if !ok || be.Code != "flow_error" || r.FlowCtrl != FlowNone {
		t.Fatalf("CallBoru = %v (flow %v), want flow_error with the signal taken", err, r.FlowCtrl)
	}
	// Negative: a quiet deferred literal evaluates after the frame.
	quiet := &FnSig{Anonymous: true, Impl: Boru([]Value{NewEvalList([]Value{group(NewInteger(4))})})}
	out, err := r.CallBoru(quiet, nil, nil)
	if err != nil || renderAll(out) != "[4]" {
		t.Fatalf("CallBoru = %s / %v, want [4]", renderAll(out), err)
	}
}
