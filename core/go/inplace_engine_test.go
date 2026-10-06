package core

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Engine-level tests for in-place compilation (inplace.go): the step loop
// run with the switch on over core/spec's notation and fixture, held to the
// legacy lane's rendering, plus white-box tests of the cold paths no core
// program reaches. lang/go/inplace_differential_test.go is the full-language
// differential; these keep core's own suite covering its own mechanism.

// inPlaceDelta is the change in the process-wide counters across one run.
// No test in this package runs in-place work in parallel, so a delta is the
// run's own.
type inPlaceDelta struct {
	Completed, StackCalls, Calls, Normalized       int64
	GenFallback, ReplanFallback, Compactions, Nops int64
	At                                             [normSites]int64
}

func inPlaceCounts() inPlaceDelta {
	d := inPlaceDelta{
		Completed: InPlaceStats.Completed.Load(), StackCalls: InPlaceStats.StackCalls.Load(),
		Calls: InPlaceStats.Calls.Load(), Normalized: InPlaceStats.Normalized.Load(),
		GenFallback: InPlaceStats.GenFallback.Load(), ReplanFallback: InPlaceStats.ReplanFallback.Load(),
		Compactions: InPlaceStats.Compactions.Load(), Nops: InPlaceStats.NopsDropped.Load(),
	}
	for i := range d.At {
		d.At[i] = InPlaceStats.NormalizedAt[i].Load()
	}
	return d
}

func (d inPlaceDelta) since(o inPlaceDelta) inPlaceDelta {
	out := inPlaceDelta{
		Completed: d.Completed - o.Completed, StackCalls: d.StackCalls - o.StackCalls,
		Calls: d.Calls - o.Calls, Normalized: d.Normalized - o.Normalized,
		GenFallback: d.GenFallback - o.GenFallback, ReplanFallback: d.ReplanFallback - o.ReplanFallback,
		Compactions: d.Compactions - o.Compactions, Nops: d.Nops - o.Nops,
	}
	for i := range out.At {
		out.At[i] = d.At[i] - o.At[i]
	}
	return out
}

// inPlaceFixture is core/spec's fixture registry plus the shapes the
// in-place paths need and the fixture lacks: a flexible two-slot word (one
// forward and one value-stack operand), a zero-result and a four-operand
// word, a nullary word that rebinds addq while it runs, and two words whose
// forward slot carries a pattern the plan does not enforce there.
func inPlaceFixture(t *testing.T, mode InPlaceMode) *Registry {
	t.Helper()
	r := coreSpecRegistry(t)
	r.RegisterNativeFunc(NativeFunc{Name: "flexq", Signatures: []Signature{{
		Args: []*Type{TInteger, TInteger},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			x, _ := AsInteger(a[0])
			y, _ := AsInteger(a[1])
			return []Value{NewInteger(x*10 + y)}, nil
		}),
		Returns: []*Type{TInteger}, BarrierPos: -1,
	}}})
	r.Register("zeroq", Signature{
		Params:  []FnParam{{Name: "a", Type: TInteger}},
		Returns: []*Type{},
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, nil
		}),
		BarrierPos: BarrierAllForward,
	})
	r.Register("fourq", Signature{
		Params:  []FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}, {Name: "c", Type: TInteger}, {Name: "d", Type: TInteger}},
		Returns: []*Type{TInteger},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			var n int64
			for _, v := range a {
				x, _ := AsInteger(v)
				n = n*10 + x
			}
			return []Value{NewInteger(n)}, nil
		}),
		BarrierPos: BarrierAllForward,
	})
	r.Register("bumpq", Signature{
		Returns: []*Type{TInteger},
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			// Shadow and restore addq: the binding is the same one, but its
			// generation moved, which is all a completion can see.
			reg.Defs.Push("addq", NewInteger(0))
			reg.Defs.Pop("addq")
			return []Value{NewInteger(2)}, nil
		}),
		BarrierPos: 0,
	})
	r.Register("fpatq", Signature{
		Params:  []FnParam{{Name: "v", Type: TAny, Pattern: ptrTypeLit(TInteger)}},
		Returns: []*Type{TAny},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{a[0]}, nil
		}),
		BarrierPos: 1,
	})
	shape := NewOrderedMap()
	shape.Set("a", NewTypeLiteral(TInteger))
	mapPattern := NewMap(shape)
	r.Register("mpatq", Signature{
		Params:  []FnParam{{Name: "m", Type: TMap, Pattern: &mapPattern}},
		Returns: []*Type{TAny},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{a[0]}, nil
		}),
		BarrierPos: 1,
	})
	if err := r.Err(); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	r.InPlace = mode
	return r
}

// inPlaceLanes runs src (core/spec `run` notation) on the legacy lane and
// with in-place compilation on, each over a fresh fixture, and returns both
// renderings and the in-place run's counter deltas.
func inPlaceLanes(t *testing.T, src string) (legacy, inPlace string, d inPlaceDelta) {
	t.Helper()
	legacy = evalCoreSpec(t, inPlaceFixture(t, InPlaceOff), "run "+src)
	before := inPlaceCounts()
	inPlace = evalCoreSpec(t, inPlaceFixture(t, InPlaceOn), "run "+src)
	return legacy, inPlace, inPlaceCounts().since(before)
}

// Every `run` row of core/spec renders exactly as the legacy lane renders it
// with in-place compilation on, and with verify mode on (which executes the
// legacy lane while it counts). The corpus is the spec of the step loop, so
// this is the mechanism's core-level differential.
func TestCoreSpecInPlace(t *testing.T) {
	entries, err := os.ReadDir(coreSpecDir)
	if err != nil {
		t.Fatalf("read %s: %v", coreSpecDir, err)
	}
	before := inPlaceCounts()
	verified := InPlaceStats.Verified.Load()
	rows := 0
	for _, ent := range entries {
		if !strings.HasSuffix(ent.Name(), ".tsv") {
			continue
		}
		for _, row := range parseCoreSpec(t, filepath.Join(coreSpecDir, ent.Name())) {
			if !strings.HasPrefix(row.expr, "run ") {
				continue
			}
			rows++
			legacy := evalCoreSpec(t, coreSpecRegistry(t), row.expr)
			for _, mode := range []InPlaceMode{InPlaceOn, InPlaceVerify} {
				r := coreSpecRegistry(t)
				r.InPlace = mode
				if got := evalCoreSpec(t, r, row.expr); got != legacy {
					t.Errorf("%s:%d mode %d: %s\n  legacy   %s\n  in place %s", row.file, row.line, mode, row.expr, legacy, got)
				}
			}
		}
	}
	TakeInPlaceDisagreements()
	d := inPlaceCounts().since(before)
	// Not vacuous: the corpus compiles forward and value-stack calls in
	// place, hands barrier commits to the legacy layout, and the verify lane
	// compares re-plans.
	if rows == 0 || d.Completed == 0 || d.StackCalls == 0 || d.Calls == 0 || d.At[NormCommit] == 0 || InPlaceStats.Verified.Load() == verified {
		t.Errorf("the corpus did not exercise the mechanism: %d rows, %+v", rows, d)
	}
}

// Programs that steer the step loop through each in-place path, each held to
// the legacy lane's rendering and to the path it is here for.
func TestInPlaceEnginePaths(t *testing.T) {
	stream := strings.Repeat("addq 1 2 ", nopCompactEvery/3+2)
	for _, tc := range []struct {
		name, src, want string
		path            func(d inPlaceDelta) bool
	}{
		{"a forward call compiles at its last arrival", "addq 1 2", "3",
			func(d inPlaceDelta) bool { return d.Completed == 1 && d.Calls == 1 && d.Nops == 3 }},
		{"a forward call with a value-stack operand", "5 flexq 1", "15",
			func(d inPlaceDelta) bool { return d.Completed == 1 && d.Nops == 3 }},
		{"a value-stack call compiles in its word's cell", "1 2 sumq", "3",
			func(d inPlaceDelta) bool { return d.StackCalls == 1 && d.Calls == 1 }},
		{"a call with no results removes its cell", "zeroq 5 7", "7",
			func(d inPlaceDelta) bool { return d.Completed == 1 }},
		{"a call with two results replaces its one cell", "pairq 3", "3 3",
			func(d inPlaceDelta) bool { return d.Completed == 1 }},
		{"four operands outgrow the record's inline storage", "fourq 1 2 3 4", "1234",
			func(d inPlaceDelta) bool { return d.Completed == 1 }},
		{"a nested group's call completes before its consumer", "addq ( addq 1 2 ) 3", "6",
			func(d inPlaceDelta) bool { return d.Completed == 2 }},
		{"a word rebound under its collection falls back to the re-plan", "addq 1 ( bumpq )", "3",
			func(d inPlaceDelta) bool { return d.GenFallback == 1 && d.At[NormFallback] == 1 && d.Completed == 0 }},
		{"a forward slot's pattern the plan skipped falls back when it fails", "fpatq 'x'", "ERROR:signature_error",
			func(d inPlaceDelta) bool { return d.ReplanFallback == 1 && d.At[NormFallback] == 1 }},
		{"a forward slot's pattern that holds compiles", "fpatq 7", "7",
			func(d inPlaceDelta) bool { return d.Completed == 1 && d.ReplanFallback == 0 }},
		{"a pending operand's pattern evaluation raises as the re-plan's does", "def m { a: p( boomq 1 ) } ; mpatq m", "ERROR:fixture_boom",
			func(d inPlaceDelta) bool { return d.ReplanFallback == 0 && d.GenFallback == 0 }},
		{"a pending operand's pattern evaluation that holds", "def m { a: 1 } ; mpatq m", "{a:1}",
			func(d inPlaceDelta) bool { return d.Completed == 1 }},
		{"a barrier commit hands the forward to the legacy layout", "bothq 1 qanyq", "ERROR:signature_error",
			func(d inPlaceDelta) bool { return d.At[NormCommit] == 1 }},
		{"a long statement stream leaves no nops behind it", stream, strings.TrimSpace(strings.Repeat("3 ", nopCompactEvery/3+2)),
			func(d inPlaceDelta) bool { return d.Compactions == 0 && d.Nops == 3*int64(nopCompactEvery/3+2) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legacy, got, d := inPlaceLanes(t, tc.src)
			if legacy != tc.want {
				t.Fatalf("the legacy lane renders %q, want %q", legacy, tc.want)
			}
			if got != legacy {
				t.Errorf("in place renders %q, legacy %q", got, legacy)
			}
			if !tc.path(d) {
				t.Errorf("the run did not take the path under test: %+v", d)
			}
		})
	}
}

// The trace shows the mechanism's steps: an arrival collected in place, the
// compile at the last one, the call cell's execution, a value-stack compile.
func TestInPlaceTraceNotes(t *testing.T) {
	r := inPlaceFixture(t, InPlaceOn)
	var buf bytes.Buffer
	toks := coreSpecAssemble(t, r, coreSpecFields("addq 1 2 1 2 sumq"))
	if _, err := RunTrace(r, toks, &buf); err != nil {
		t.Fatalf("trace run: %v", err)
	}
	out := buf.String()
	for _, note := range []string{"collect addq 1/2", "compile addq", "call addq", "compile sumq", "call sumq"} {
		if !strings.Contains(out, note) {
			t.Errorf("the trace lacks %q", note)
		}
	}
	// The legacy lane's trace has none of them.
	buf.Reset()
	r = inPlaceFixture(t, InPlaceOff)
	if _, err := RunTrace(r, coreSpecAssemble(t, r, coreSpecFields("addq 1 2")), &buf); err != nil {
		t.Fatalf("trace run: %v", err)
	}
	if strings.Contains(buf.String(), "compile ") {
		t.Error("the legacy lane traced an in-place compile")
	}
}

// inPlaceForward is a pending in-place forward marker for flexq.
func inPlaceForward(funcIdx, collected, expected, stack int) Value {
	sig := &Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: -1}
	return NewForward(ForwardInfo{
		FuncName: "flexq", Sig: sig, FuncIndex: funcIdx, CollectedArgs: collected,
		ExpectedArgs: expected, StackArgs: stack, InPlace: true,
	})
}

// A collected value's reach-collapse tag is spent on arrival, in place as on
// the legacy lane — and an arrival short of the last only advances.
func TestInPlaceArrivalClearsReachTag(t *testing.T) {
	tagged := NewInteger(1)
	tagged.ReachGroup = true
	e := engWithTape(t, []Value{NewWord("flexq"), inPlaceForward(0, 0, 2, 0), tagged}, 2)
	fwd, _ := AsForward(e.Tape.At(1))
	if err := e.arriveInPlace(fwd, 1, 2); err != nil {
		t.Fatalf("arrival: %v", err)
	}
	if e.Tape.At(2).ReachGroup {
		t.Error("the collected value kept its reach tag")
	}
	if got, _ := AsForward(e.Tape.At(1)); got.CollectedArgs != 1 || e.Pointer != 3 || IsCall(e.Tape.At(2)) {
		t.Errorf("a first arrival of two collects and advances: %+v, pointer %d", got, e.Pointer)
	}
}

// The loop region's pass normalises every in-place forward in its range to
// the legacy layout and leaves everything else where it is.
func TestInPlaceNormalizeForwardsIn(t *testing.T) {
	legacy := fwdMarker("cadd", 0, 0, 0)
	e := engWithTape(t, []Value{legacy, NewInteger(9), NewWord("flexq"), inPlaceForward(2, 1, 2, 0), NewInteger(1)}, 5)
	before := InPlaceStats.NormalizedAt[NormLoopRegion].Load()
	e.normalizeForwardsIn(0, 5)
	if InPlaceStats.NormalizedAt[NormLoopRegion].Load() != before+1 {
		t.Error("one in-place forward was in range")
	}
	want := []string{"forward", "9", "1", "flexq", "forward"}
	for i, w := range want {
		v := e.Tape.At(i)
		got := v.String()
		if IsForward(v) {
			got = "forward"
		}
		if got != w && !strings.Contains(got, w) {
			t.Errorf("cell %d = %s, want %s", i, got, w)
		}
	}
	if f, _ := AsForward(e.Tape.At(4)); f.InPlace || f.FuncIndex != 3 {
		t.Errorf("the normalised forward: %+v", f)
	}
	// No forward on the tape: nothing to do.
	e = engWithTape(t, []Value{NewInteger(1), nopCell}, 2)
	e.normalizeForwardsIn(0, 2)
	if e.Tape.Len() != 2 {
		t.Error("a forward-free range changed")
	}
}

// A word dispatched under a pending in-place forward does not see that
// forward's claims — its word, the operands after its marker, its stack
// operands — as the legacy layout's exclusions hide them there.
func TestInPlaceEffectiveResolvedExcludesClaims(t *testing.T) {
	// 9 is free; 5 is flexq's value-stack operand, 2 its collected one.
	inPlace := engWithTape(t, []Value{NewInteger(9), NewInteger(5), NewWord("flexq"), inPlaceForward(2, 1, 2, 1), NewInteger(2)}, 5)
	legacyFwd := NewForward(ForwardInfo{FuncName: "flexq", FuncIndex: 3, CollectedArgs: 1, StackArgs: 1})
	legacy := engWithTape(t, []Value{NewInteger(9), NewInteger(5), NewInteger(2), NewWord("flexq"), legacyFwd}, 5)
	if got, want := renderAll(inPlace.EffectiveResolved()), renderAll(legacy.EffectiveResolved()); got != want || got != "9" {
		t.Errorf("in place %q, legacy %q, want 9", got, want)
	}
	// Read as a legacy forward, the same in-place layout hides the wrong cells.
	f, _ := AsForward(inPlace.Tape.At(3))
	f.InPlace = false
	inPlace.Tape.Set(3, NewForward(f))
	if got := renderAll(inPlace.EffectiveResolved()); got == "9" {
		t.Error("the in-place exclusion is not doing the work")
	}
}

// A forward terminated under a pending in-place forward is retried in stack
// form over what that forward has not claimed.
func TestInPlaceCurryOrStackExcludesClaims(t *testing.T) {
	r := inPlaceFixture(t, InPlaceOn)
	outer := NewForward(ForwardInfo{FuncName: "addq", FuncIndex: 1, CollectedArgs: 1, ExpectedArgs: 2, InPlace: true})
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(9), NewWord("addq"), outer, NewInteger(1), NewWord("negq")}, StackHeadroom)
	e.curryOrStack(4, 0, 0)
	w, err := AsWord(e.Tape.At(4))
	if err != nil || !w.ForceStack || e.Pointer != 4 {
		t.Errorf("negq must retry in stack form over the free 9: %v, pointer %d", e.Tape.At(4), e.Pointer)
	}
	// With the claimed 1 the only value below, nothing is free: no retry.
	e.Tape = NewTape([]Value{NewWord("addq"), NewForward(ForwardInfo{FuncName: "addq", FuncIndex: 0, CollectedArgs: 1, ExpectedArgs: 2, InPlace: true}), NewInteger(1), NewWord("negq")}, StackHeadroom)
	e.curryOrStack(3, 0, 0)
	if w, err := AsWord(e.Tape.At(3)); err == nil && w.ForceStack {
		t.Error("negq retried in stack form over a value the outer forward claimed")
	}
}

// An in-place call's tail probe declines when its span holds anything but
// nops below the cell — a mark a stack operand was claimed across.
func TestInPlaceTailProbeSpan(t *testing.T) {
	call := Value{Parent: TInternal, Data: newCallInfo("f", nil, 1, 0)}
	e := engWithTape(t, []Value{NewInteger(5), NewMark("m"), nopCell, call}, 3)
	match := &MatchResult{InPlace: true, SpanLo: 0}
	if scan, ok := e.probeTailCallFor(match, nil, 1); ok || scan.RCIdx != -1 {
		t.Error("an interleaved mark must decline the probe")
	}
	e = engWithTape(t, []Value{nopCell, nopCell, call}, 2)
	if _, ok := e.probeTailCallFor(&MatchResult{InPlace: true, SpanLo: 0}, nil, 1); ok {
		t.Error("no frame tail follows the cell: the probe finds no tail call")
	}
}

// planHolds judges the arrived operands as the legacy re-plan does.
func TestInPlacePlanHolds(t *testing.T) {
	e := engWithTape(t, []Value{NewWord("x"), NewString("s"), NewTypeLiteral(TInteger), NewInteger(1)}, 4)
	quoted := func(t *Type) *Signature {
		return &Signature{Args: []*Type{t}, QuoteArgs: map[int]bool{0: true}}
	}
	for _, tc := range []struct {
		name string
		sig  *Signature
		at   int
		want bool
	}{
		{"a /q word into a slot that takes its name", quoted(TAny), 0, true},
		{"a /q word into a slot that cannot take a name", quoted(TInteger), 0, false},
		{"a value of the slot's type", &Signature{Args: []*Type{TInteger}}, 3, true},
		{"a value of another type", &Signature{Args: []*Type{TInteger}}, 1, false},
		{"a type literal where the slot takes values", &Signature{Args: []*Type{TInteger}}, 2, false},
		{"a type literal into a type-arg slot", &Signature{Args: []*Type{TInteger}, TypeArgs: map[int]bool{0: true}}, 2, true},
	} {
		if got := e.planHolds(tc.sig, []int{tc.at}, nil); got != tc.want {
			t.Errorf("%s: planHolds = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A loop body that takes a value-stack operand from below its mark claims
// it across the mark, so the call's span is not all nops and its nops stay
// when the call runs; the statement compaction drops the ones the iterations
// pile up below the mark.
func TestInPlaceLoopLeftoverNops(t *testing.T) {
	const n = nopCompactEvery // iterations: ~3 leftover nops each
	run := func(mode InPlaceMode) string {
		r := inPlaceFixture(t, mode)
		body := []Value{NewWord("addq"), NewInteger(100)}
		cont := &ForCont{Registry: r, IterName: "i", Current: 0, End: n, Step: 1, Body: body}
		id := NextMarkID()
		var toks []Value
		for k := 1; k <= n; k++ {
			toks = append(toks, NewInteger(int64(k)))
		}
		toks = append(append(append(toks, NewMark(id)), body...), NewMoveCont(id, "for", cont))
		out, err := NewTop(r).Run(toks)
		if err != nil {
			t.Fatalf("mode %d: %v", mode, err)
		}
		return renderAll(out)
	}
	before := InPlaceStats.Compactions.Load()
	got, legacy := run(InPlaceOn), run(InPlaceOff)
	if got != legacy || !strings.HasPrefix(legacy, fmt.Sprint(100+n)) {
		t.Errorf("in place %q, legacy %q", got, legacy)
	}
	if InPlaceStats.Compactions.Load() == before {
		t.Error("the leftover nops below the mark were never compacted")
	}
}

// A loop iteration's collection steps over the nops its body's in-place
// calls left in the region.
func TestInPlaceLoopRegion(t *testing.T) {
	run := func(mode InPlaceMode) string {
		r := inPlaceFixture(t, mode)
		body := []Value{NewWord("addq"), NewInteger(1), NewInteger(2)}
		cont := &ForCont{Registry: r, IterName: "i", Current: 0, End: 2, Step: 1, Body: body}
		id := NextMarkID()
		toks := append(append([]Value{NewMark(id)}, body...), NewMoveCont(id, "for", cont))
		out, err := NewTop(r).Run(toks)
		if err != nil {
			t.Fatalf("mode %d: %v", mode, err)
		}
		return renderAll(out)
	}
	before := InPlaceStats.Completed.Load()
	if got, legacy := run(InPlaceOn), run(InPlaceOff); got != legacy || got != "3 | 3" {
		t.Errorf("in place %q, legacy %q, want 3 | 3", got, legacy)
	}
	if InPlaceStats.Completed.Load()-before != 2 {
		t.Error("each iteration's call compiles in place")
	}
}

// The mechanism's point, pinned in tape edits: a fully forward call of n
// operands parks its marker (the one Insert) and otherwise only overwrites
// cells — no operand is removed and re-inserted, and the one Splice is the
// run end's nop drop — where the legacy completion moves every operand
// before the word and splices the result over the span.
func TestInPlaceTapeEdits(t *testing.T) {
	stats := func(mode InPlaceMode, src string) TapeStats {
		r := inPlaceFixture(t, mode)
		e := NewTop(r)
		if _, err := e.Run(coreSpecAssemble(t, r, coreSpecFields(src))); err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		return e.Tape.Stats()
	}
	for _, src := range []string{"addq 1 2", "fourq 1 2 3 4"} {
		on, off := stats(InPlaceOn, src), stats(InPlaceOff, src)
		if on.Inserts != 1 || on.Removes != 0 || on.Splices != 1 {
			t.Errorf("%s in place: %+v", src, on)
		}
		if off.Removes == 0 || off.Inserts <= on.Inserts || off.Moved <= on.Moved {
			t.Errorf("%s legacy: %+v, in place %+v", src, off, on)
		}
	}
}

// A call with no results leaves the pointer on the cell after it, as the
// legacy splice of its span does: a signal it raises with no loop to take it
// (a `break` outside any loop) reports there, on both lanes — never at the
// nop, which has no source position.
func TestInPlaceZeroResultPosition(t *testing.T) {
	report := func(mode InPlaceMode) string {
		r := inPlaceFixture(t, mode)
		r.Register("flowq", Signature{
			Params:  []FnParam{{Name: "a", Type: TInteger}},
			Returns: []*Type{},
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				reg.FlowCtrl = FlowBreak
				return nil, nil
			}),
			BarrierPos: BarrierAllForward,
		})
		at := func(v Value, col int) Value {
			v.SetPos(SrcPos{Row: 1, Col: col})
			return v
		}
		_, err := NewTop(r).Run([]Value{at(NewWord("flowq"), 1), at(NewInteger(1), 7), at(NewInteger(7), 9)})
		var be *BoruError
		if !errors.As(err, &be) || be.Code != "flow_error" {
			t.Fatalf("mode %d: want a flow_error, got %v", mode, err)
		}
		return fmt.Sprintf("%d:%d", be.Row, be.Col)
	}
	if on, off := report(InPlaceOn), report(InPlaceOff); on != off || on != "1:9" {
		t.Errorf("in place reports at %s, legacy at %s, want 1:9", on, off)
	}
}

// The step loop never lands on a nop the mechanism left — every nop is
// written behind the pointer — but a nop stepped is skipped, never pushed.
func TestInPlaceNopStepped(t *testing.T) {
	e := engWithTape(t, []Value{nopCell, NewInteger(1)}, 0)
	if ok, err := e.stepInPlaceCell(e.Tape.At(0)); !ok || err != nil || e.Pointer != 1 {
		t.Errorf("a nop at the pointer: ok %v, err %v, pointer %d", ok, err, e.Pointer)
	}
	if ok, _ := e.stepInPlaceCell(e.Tape.At(1)); ok {
		t.Error("a value is no in-place cell")
	}
}
