package core

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Core-suite pins for the DRIVEN loop protocol (loop.go): a native looping
// construct's iterations run on the tape as sealed regions, break/continue
// pass through the loop, a body error carries the driver's attribution,
// the driver's own errors end the loop at its word, and every exit — the
// move, a signal, a fault — leaves the context stack as it found it.

// tlDriver is a LoopDriver over a list of elements: one element per
// iteration, the body's top kept, the kept values listed at the end. The
// *At fields make a given iteration's Next or Collect fail, finishErr makes
// Finish fail.
type tlDriver struct {
	reg          *Registry
	body         Value
	elems        []Value
	kept         []Value
	in           [1]Value
	nextErrAt    int
	collectErrAt int
	finishErr    error
}

func (d *tlDriver) Next(it int) ([]Value, Value, bool, error) {
	if it == d.nextErrAt {
		return nil, Value{}, false, d.reg.BoruError("tl_error", "next failed", "tl")
	}
	if it >= len(d.elems) {
		return nil, Value{}, false, nil
	}
	d.in[0] = d.elems[it]
	return d.in[:], d.body, true, nil
}

func (d *tlDriver) Collect(it int, res []Value) error {
	if it == d.collectErrAt {
		return d.reg.BoruError("tl_error", "collect failed", "tl")
	}
	if len(res) == 0 {
		return d.reg.BoruError("tl_error", fmt.Sprintf("tl: element %d: no result", it), "tl")
	}
	d.kept = append(d.kept, res[len(res)-1])
	return nil
}

func (d *tlDriver) Finish() ([]Value, error) {
	if d.finishErr != nil {
		return nil, d.finishErr
	}
	return []Value{NewList(d.kept)}, nil
}

func (d *tlDriver) Describe() string { return fmt.Sprintf("tl %d/%d", len(d.kept)+1, len(d.elems)) }

func (d *tlDriver) WrapError(it int, err error) error {
	return fmt.Errorf("tl: element %d: %w", it, err)
}

// loopReg is nurReg plus the driven-loop words: tl (the plain loop) and
// its failing variants, each `name [body] [data]`.
func loopReg(t *testing.T) *Registry {
	t.Helper()
	r := nurReg(t)
	word := func(name string, tweak func(*tlDriver)) {
		r.RegisterNativeFunc(NativeFunc{Name: name, Signatures: []Signature{{
			Args: []*Type{TList, TList}, NoEvalArgs: map[int]bool{0: true},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				lst, _ := AsList(args[1])
				d := &tlDriver{reg: reg, body: args[0], elems: lst.Slice(), nextErrAt: -1, collectErrAt: -1}
				if tweak != nil {
					tweak(d)
				}
				return StartLoop(reg, "tl", d, nil)
			}),
			Returns: []*Type{TList}, BarrierPos: -1,
		}}})
	}
	word("tl", nil)
	r.RegisterNativeFunc(NativeFunc{Name: "tdrop", Signatures: []Signature{{
		Args:    []*Type{TAny},
		Impl:    Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return nil, nil }),
		Returns: []*Type{}, BarrierPos: 0,
	}}})
	word("tlnext0", func(d *tlDriver) { d.nextErrAt = 0 })
	word("tlnext1", func(d *tlDriver) { d.nextErrAt = 1 })
	word("tlcol1", func(d *tlDriver) { d.collectErrAt = 1 })
	word("tlfin", func(d *tlDriver) { d.finishErr = r.BoruError("tl_error", "finish failed", "tl") })
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	return r
}

func ints(ns ...int64) Value {
	vs := make([]Value, len(ns))
	for i, n := range ns {
		vs[i] = NewInteger(n)
	}
	return NewList(vs)
}

func words(ws ...string) Value {
	vs := make([]Value, len(ws))
	for i, w := range ws {
		vs[i] = NewWord(w)
	}
	return NewList(vs)
}

// tlCall is `name [body] data` at row 4.
func tlCall(name string, body, data Value) []Value {
	w := WithPos(NewWord(name), Value{pos: &SrcPos{Row: 4, Col: 2}})
	return []Value{w, body, data}
}

func TestDrivenLoopRunsOnTheTape(t *testing.T) {
	r := loopReg(t)
	e := NewTop(r)
	var notes []string
	e.SetTrace(func(_, _ int, _ []Value, note string) {
		if note != "" {
			notes = append(notes, note)
		}
	})
	depth := r.Contexts.Depth()
	out, err := e.Run(tlCall("tl", words("cdub"), ints(1, 2, 3)))
	if err != nil || renderAll(out) != "[2 4 6]" {
		t.Fatalf("Run = %s / %v, want [2 4 6]", renderAll(out), err)
	}
	if r.Contexts.Depth() != depth {
		t.Fatalf("context depth %d after the loop, want %d", r.Contexts.Depth(), depth)
	}
	joined := strings.Join(notes, "\n")
	for _, want := range []string{"loop tl 1/3", "loop next ", "tl 2/3", "tl 3/3", "loop done tl"} {
		if !strings.Contains(joined, want) {
			t.Errorf("trace notes lack %q:\n%s", want, joined)
		}
	}
	// The body's stack is SEALED to its inputs: a value beneath the loop
	// is not the body's to take.
	_, err = NewTop(r).Run(append([]Value{NewInteger(10)}, tlCall("tl", words("cadd"), ints(1))...))
	if err == nil || !strings.Contains(err.Error(), "tl: element 0:") {
		t.Fatalf("sealed region: err = %v, want the body's dispatch failure attributed to element 0", err)
	}
	// A Function-valued element is inert on placement: the body sees the
	// value, which is not applied.
	fnv := NewFunction(FnDefInfo{Name: "fv"})
	out, err = NewTop(r).Run(tlCall("tl", NewList([]Value{}), NewList([]Value{fnv})))
	if err != nil || len(out) != 1 {
		t.Fatalf("inert input: %s / %v", renderAll(out), err)
	}
	if lst, _ := AsList(out[0]); lst.Len() != 1 || !lst.Get(0).Parent.Equal(TFunction) {
		t.Fatalf("inert input: the element was not kept as the fn value: %s", renderAll(out))
	}
}

func TestDrivenLoopEmptyAndNestedAndContext(t *testing.T) {
	r := loopReg(t)
	// No iteration: the driver's results are the handler's results.
	out, err := NewTop(r).Run(tlCall("tl", words("cdub"), ints()))
	if err != nil || renderAll(out) != "[]" {
		t.Fatalf("empty: %s / %v", renderAll(out), err)
	}
	// Nested loops: the inner loop's region is the outer body.
	inner := append([]Value{}, tlCall("tl", words("cdub"), ints(1, 2))...)
	out, err = NewTop(r).Run(tlCall("tl", NewList(inner), ints(0)))
	if err != nil || renderAll(out) != "[[2 4]]" {
		t.Fatalf("nested: %s / %v, want the inner loop's list as the one kept value", renderAll(out), err)
	}
	// Each iteration runs under its own context layer: a write inside the
	// body is the iteration's, and the layer is popped at the move.
	depth := r.Contexts.Depth()
	e := NewTop(r)
	var depthInBody int
	r.RegisterNativeFunc(NativeFunc{Name: "cdepth", Signatures: []Signature{{
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			depthInBody = reg.Contexts.Depth()
			return []Value{NewInteger(1)}, nil
		}),
		Returns: []*Type{TInteger}, BarrierPos: -1,
	}}})
	if _, err := e.Run(tlCall("tl", words("cdepth"), ints(1))); err != nil {
		t.Fatal(err)
	}
	// Run pushes one layer for the run, the iteration one more.
	if depthInBody != depth+2 || r.Contexts.Depth() != depth {
		t.Fatalf("context depth in body %d (want %d), after %d (want %d)", depthInBody, depth+2, r.Contexts.Depth(), depth)
	}
}

func TestDrivenLoopDriverErrorsEndTheLoopAtItsWord(t *testing.T) {
	r := loopReg(t)
	for _, c := range []struct{ word, detail string }{
		{"tlnext0", "next failed"},
		{"tlnext1", "next failed"},
		{"tlcol1", "collect failed"},
		{"tlfin", "finish failed"},
	} {
		depth := r.Contexts.Depth()
		_, err := NewTop(r).Run(tlCall(c.word, words("cdub"), ints(1, 2, 3)))
		var be *BoruError
		if !errors.As(err, &be) || be.Code != "tl_error" || be.Detail != c.detail {
			t.Fatalf("%s: err = %v, want tl_error %q", c.word, err, c.detail)
		}
		if strings.Contains(err.Error(), "tl: element") {
			t.Fatalf("%s: the driver's own error must not be attributed to a body: %v", c.word, err)
		}
		if be.Row != 4 {
			t.Fatalf("%s: positioned at row %d, want the word's row 4", c.word, be.Row)
		}
		if r.Contexts.Depth() != depth {
			t.Fatalf("%s: context depth %d after the error, want %d", c.word, r.Contexts.Depth(), depth)
		}
	}
	// A body that nets nothing is the construct's own error, at the word.
	_, err := NewTop(r).Run(tlCall("tl", words("tdrop"), ints(1)))
	var be *BoruError
	if !errors.As(err, &be) || be.Code != "tl_error" || be.Row != 4 {
		t.Fatalf("no result: err = %v", err)
	}
}

func TestDrivenLoopBodyFaultIsAttributedAndUnwound(t *testing.T) {
	r := loopReg(t)
	depth := r.Contexts.Depth()
	_, err := NewTop(r).Run(tlCall("tl", words("nosuch_word"), ints(1, 2)))
	if err == nil || !strings.HasPrefix(err.Error(), "tl: element 0: ") {
		t.Fatalf("err = %v, want `tl: element 0: …`", err)
	}
	var be *BoruError
	if !errors.As(err, &be) {
		t.Fatalf("the wrapped error must keep the BoruError: %v", err)
	}
	if r.Contexts.Depth() != depth {
		t.Fatalf("context depth %d after the fault, want %d", r.Contexts.Depth(), depth)
	}
	// Nested: innermost first, as each handler wrapped its sub-engine's error.
	inner := tlCall("tl", words("nosuch_word"), ints(5))
	_, err = NewTop(r).Run(tlCall("tl", NewList(inner), ints(1)))
	if err == nil || !strings.HasPrefix(err.Error(), "tl: element 0: tl: element 0: ") {
		t.Fatalf("nested err = %v", err)
	}
	if r.Contexts.Depth() != depth {
		t.Fatalf("nested: context depth %d, want %d", r.Contexts.Depth(), depth)
	}
}

// forAround wraps body tokens in a two-iteration `for` loop over i (the
// basic RunForLoop shape), the index installed for the first iteration.
func forAround(r *Registry, body ...Value) []Value {
	InstallDef(r, "i", NewInteger(0))
	cont := &Loop{Registry: r, Word: "for", Iter: 1, Count: 2, IterName: "i", Current: 0, End: 2, Step: 1,
		Body: append([]Value(nil), body...), IterDepth: r.Defs.Depth("i")}
	toks := []Value{NewMark("forL", body...)}
	toks = append(toks, body...)
	return append(toks, NewMoveCont("forL", "for loop", cont))
}

func TestDrivenLoopIsTransparentToBreakAndContinue(t *testing.T) {
	r := loopReg(t)
	depth := r.Contexts.Depth()
	// break inside the body breaks the ENCLOSING for, not the driven loop.
	out, err := NewTop(r).Run(forAround(r, tlCall("tl", words("nbrk"), ints(1, 2))...))
	if err != nil || renderAll(out) != "" {
		t.Fatalf("break: %s / %v, want nothing", renderAll(out), err)
	}
	if r.Contexts.Depth() != depth {
		t.Fatalf("break: context depth %d, want %d", r.Contexts.Depth(), depth)
	}
	// continue abandons the for's iteration, the driven loop with it.
	body := append(tlCall("tl", words("ncont"), ints(1, 2)), NewInteger(7))
	out, err = NewTop(r).Run(forAround(r, body...))
	if err != nil || renderAll(out) != "" {
		t.Fatalf("continue: %s / %v, want nothing", renderAll(out), err)
	}
	if r.Contexts.Depth() != depth {
		t.Fatalf("continue: context depth %d, want %d", r.Contexts.Depth(), depth)
	}
	// With no loop around it, the signal reaches the top as it did when
	// the handler returned nothing.
	wantFlowError(t, r, tlCall("tl", words("nbrk"), ints(1, 2)), "break")
	if r.Contexts.Depth() != depth {
		t.Fatalf("top: context depth %d, want %d", r.Contexts.Depth(), depth)
	}
	// A signal a residual literal lets out at the move (NUR358) passes
	// through the loop the same way.
	wantFlowError(t, r, tlCall("tl", NewList([]Value{NewEvalList([]Value{NewWord("nbrk")})}), ints(1)), "break")
	if r.Contexts.Depth() != depth {
		t.Fatalf("residual escape: context depth %d, want %d", r.Contexts.Depth(), depth)
	}
	// A loop inside a forward-collected paren group: the group is abandoned
	// whole, the loop with it (the collecting word then fails over nothing,
	// as it does for any signal escaping its group).
	group := []Value{NewWord("cneg"), NewOpenParen()}
	group = append(group, tlCall("tl", words("nbrk"), ints(1))...)
	group = append(group, NewCloseParen())
	if _, err := NewTop(r).Run(group); err == nil {
		t.Fatal("group: expected the collecting word's failure")
	}
	r.TakeFlow()
	if r.Contexts.Depth() != depth {
		t.Fatalf("group: context depth %d, want %d", r.Contexts.Depth(), depth)
	}
	// The tape-level abandonment clamps its bounds and skips a move that
	// is not a live driven loop's: a plain move, a loop whose mark was
	// never stepped.
	e := NewTop(r)
	stale := &Loop{Driver: &tlDriver{}}
	e.Tape = NewTape([]Value{NewMove("p", "plain"), NewMoveCont("never", "tl loop", stale)}, StackHeadroom)
	e.marks = map[string]bool{"x": true}
	e.abandonDrivenLoops(-5, 99)
	if len(e.marks) != 1 {
		t.Fatalf("abandon over no live loop touched the marks: %v", e.marks)
	}
}

func TestDriveLoopFromGo(t *testing.T) {
	r := loopReg(t)
	mk := func() *tlDriver {
		return &tlDriver{reg: r, body: words("cdub"), elems: []Value{NewInteger(1), NewInteger(2)}, nextErrAt: -1, collectErrAt: -1}
	}
	out, err := DriveLoop(r, mk(), nil)
	if err != nil || renderAll(out) != "[2 4]" {
		t.Fatalf("DriveLoop = %s / %v", renderAll(out), err)
	}
	// An explicit invoke.
	calls := 0
	out, err = DriveLoop(r, mk(), func(body Value, inputs []Value) ([]Value, error) {
		calls++
		return RunResolved(r, inputs, BodyTokens(body))
	})
	if err != nil || renderAll(out) != "[2 4]" || calls != 2 {
		t.Fatalf("DriveLoop invoke = %s / %v (%d calls)", renderAll(out), err, calls)
	}
	// The driver's errors.
	d := mk()
	d.nextErrAt = 1
	if _, err := DriveLoop(r, d, nil); err == nil || !strings.Contains(err.Error(), "next failed") {
		t.Fatalf("next error: %v", err)
	}
	d = mk()
	d.collectErrAt = 0
	if _, err := DriveLoop(r, d, nil); err == nil || !strings.Contains(err.Error(), "collect failed") {
		t.Fatalf("collect error: %v", err)
	}
	// A body error is attributed.
	d = mk()
	d.body = words("nosuch_word")
	if _, err := DriveLoop(r, d, nil); err == nil || !strings.HasPrefix(err.Error(), "tl: element 0: ") {
		t.Fatalf("body error: %v", err)
	}
	// An escaping signal ends the loop with no result.
	d = mk()
	d.body = words("nbrk")
	out, err = DriveLoop(r, d, nil)
	if err != nil || out != nil || r.FlowCtrl != FlowBreak {
		t.Fatalf("escape: %s / %v / %v", renderAll(out), err, r.FlowCtrl)
	}
	r.TakeFlow()
	// StartLoop under an Invoker (the VM lane) drives from Go.
	r.Invoker = func(reg *Registry, body Value, inputs []Value) ([]Value, error) {
		return RunResolved(reg, inputs, BodyTokens(body))
	}
	out, err = StartLoop(r, "tl", mk(), nil)
	r.Invoker = nil
	if err != nil || renderAll(out) != "[2 4]" {
		t.Fatalf("StartLoop under Invoker = %s / %v", renderAll(out), err)
	}
}

func TestLoopAnnotations(t *testing.T) {
	var nilLoop *Loop
	if nilLoop.Describe() != "" {
		t.Fatal("nil loop describes as nothing")
	}
	counted := &Loop{Word: "for", Iter: 2, Count: 5, IterName: "i", Current: 1}
	if got := counted.Describe(); got != "for i=1 2/5" {
		t.Fatalf("counted: %q", got)
	}
	unknown := &Loop{IterName: "i", Current: 3, Count: -1}
	if got := unknown.Describe(); got != "for i=3" {
		t.Fatalf("unknown count: %q", got)
	}
	while := &Loop{WhileCond: []Value{}, Iter: 1}
	if got := while.Describe(); got != "while #1 cond" {
		t.Fatalf("while cond: %q", got)
	}
	while.WhileInBody = true
	if got := while.Describe(); got != "while #1 body" {
		t.Fatalf("while body: %q", got)
	}
	driven := &Loop{Driver: &tlDriver{elems: make([]Value, 3)}}
	if got := driven.Describe(); got != "tl 1/3" || driven.word() != "loop" {
		t.Fatalf("driven: %q / %q", got, driven.word())
	}
	// The renders carry the annotation.
	mark := NewLoopMark("m7", driven)
	if got := mark.String(); got != "mark(m7,tl 1/3)" {
		t.Fatalf("mark render: %q", got)
	}
	move := NewMoveCont("m7", "tl loop", driven)
	if got := move.String(); got != "move(m7,tl loop,tl 1/3)" {
		t.Fatalf("move render: %q", got)
	}
	if got := NewMove("m8", "plain").String(); got != "move(m8,plain)" {
		t.Fatalf("plain move render: %q", got)
	}
	// The body enters the region as the list's own elements, or as its
	// one token when it is not a list (a Function value).
	lp := &Loop{}
	if toks := lp.bodyTokens(words("cdub", "cadd")); len(toks) != 2 {
		t.Fatalf("list body tokens: %v", toks)
	}
	if toks := lp.bodyTokens(NewInteger(5)); len(toks) != 1 || toks[0].String() != "5" {
		t.Fatalf("one-token body: %v", toks)
	}
	// The region predicates.
	if openArgSpan(NewLoopOpen(driven, 2)) != 2 || openArgSpan(NewFrameOpenSpan(fnValueFrameMeta, 3)) != 3 || openArgSpan(NewOpenParen()) != 0 {
		t.Fatal("openArgSpan")
	}
	if IsLoopRegion(nil) || IsLoopRegion([]Value{NewInteger(1)}) || IsLoopRegion([]Value{NewMark("p")}) || !IsLoopRegion([]Value{mark}) {
		t.Fatal("IsLoopRegion")
	}
	// The sealing paren's payload is never type content.
	if (LoopOpenInfo{}).IsTypeContent(nil) {
		t.Fatal("LoopOpenInfo is not type content")
	}
}

// TestDrivenLoopIterationBudget: every iteration runs on a step budget of its
// own and its steps are never charged to the run holding the loop — the
// sub-engine protocol's metering (the VM's enterBodyUnit resets per body the
// same way). A runaway INSIDE one iteration still trips the limit, attributed
// to the element.
func TestDrivenLoopIterationBudget(t *testing.T) {
	r := loopReg(t)
	body := words("cdub", "cdub", "cdub")
	data := ints(1, 2, 3, 4, 5, 6, 7, 8)
	// Eight iterations of ~8 steps each: far past a 24-step budget in total,
	// well within it each; the run's own tail still fits after the loop.
	prog := append(tlCall("tl", body, data), NewInteger(1), NewWord("cdub"), NewWord("cdub"), NewWord("cdub"))
	e := NewTop(r)
	defer e.StepBudget(24, 0)()
	out, err := e.Run(prog)
	if err != nil || renderAll(out) != "[8 16 24 32 40 48 56 64] | 8" {
		t.Fatalf("budgeted loop = %s / %v", renderAll(out), err)
	}
	// A body that outruns the budget on its own is the element's fault.
	long := make([]string, 40)
	for i := range long {
		long[i] = "cdub"
	}
	e2 := NewTop(r)
	defer e2.StepBudget(24, 0)()
	_, err = e2.Run(tlCall("tl", words(long...), ints(1)))
	if err == nil || !strings.HasPrefix(err.Error(), "tl: element 0: ") || !strings.Contains(err.Error(), "evaluation_limit") {
		t.Fatalf("runaway body: %v", err)
	}
	// Inside a forward-collected paren group — a budget scope of its own —
	// the iterations are metered the same way.
	group := []Value{NewWord("tl"), words("cdub"), NewOpenParen()}
	group = append(group, tlCall("tl", body, data)...)
	group = append(group, NewCloseParen())
	e4 := NewTop(r)
	defer e4.StepBudget(24, 0)()
	out, err = e4.Run(group)
	if err != nil || renderAll(out) != "[16 32 48 64 80 96 112 128]" {
		t.Fatalf("budgeted loop in a group = %s / %v", renderAll(out), err)
	}
	// The run's OWN runaway after a loop still trips: the loop's steps are
	// uncharged, the tail's are not.
	tail := make([]Value, 0, 40)
	for i := 0; i < 30; i++ {
		tail = append(tail, NewWord("cdub"))
	}
	e3 := NewTop(r)
	defer e3.StepBudget(24, 0)()
	_, err = e3.Run(append(append(tlCall("tl", body, ints(1)), NewInteger(1)), tail...))
	if err == nil || !strings.Contains(err.Error(), "evaluation_limit") || strings.Contains(err.Error(), "tl: element") {
		t.Fatalf("runaway tail: %v", err)
	}
}
