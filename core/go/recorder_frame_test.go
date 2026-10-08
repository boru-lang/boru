package core

import "testing"

// frameRec records the events a Recorder receives, so a test can assert on
// WHAT was reported rather than only how many.
type frameRec struct {
	lits  []Value
	calls []struct {
		name           string
		arity, returns int
	}
}

func (r *frameRec) OnPushLit(v Value) { r.lits = append(r.lits, v) }
func (r *frameRec) OnCall(name string, arity, returns int) {
	r.calls = append(r.calls, struct {
		name           string
		arity, returns int
	}{name, arity, returns})
}

// inFnFrame is what keeps a callee's internals out of the caller's recorded
// strict-stack form. It answers from the TAPE rather than from a counter,
// precisely so that it cannot latch on when a frame is abandoned instead of
// closed (TCO replacement, an error unwind, the step limit) — so the arms
// worth pinning are the paren-matching ones, which is where a hand-rolled
// scan goes wrong.
//
// Covered here rather than only through lang/go's round-trip suite because
// core/go is gated at a 100% floor by its OWN suite (`make cover-gate-core`,
// core/go/CLAUDE.md), and core registers no `def`/`fn` to build a real frame
// from source.
func TestInFnFrame(t *testing.T) {
	frameOpen := NewFrameOpen(&FnFrameMeta{})
	userOpen := NewOpenParen()
	closeP := NewCloseParen()
	one := NewInteger(1)

	cases := []struct {
		name    string
		tape    []Value
		pointer int
		want    bool
		why     string
	}{
		{
			name: "empty prefix", tape: []Value{one}, pointer: 0, want: false,
			why: "nothing precedes the pointer, so no frame can enclose it",
		},
		{
			name: "top level after a value", tape: []Value{one, one}, pointer: 1, want: false,
			why: "an ordinary top-level literal is the caller's own program",
		},
		{
			name: "directly inside a frame", tape: []Value{frameOpen, one}, pointer: 1, want: true,
			why: "the frame's open paren is unmatched before the pointer",
		},
		{
			name: "inside a USER paren only", tape: []Value{userOpen, one}, pointer: 1, want: false,
			why: "a grouping paren is the caller's syntax, not a callee boundary",
		},
		{
			name: "user paren nested in a frame", tape: []Value{frameOpen, userOpen, one}, pointer: 2, want: true,
			why: "the frame still encloses it — the inner paren is unmatched too but is not a frame",
		},
		{
			name: "after a CLOSED frame", tape: []Value{frameOpen, one, closeP, one}, pointer: 3, want: false,
			why: "the close paren matches the frame open, so the pointer is back at top level",
		},
		{
			name: "after a closed frame, still inside an outer one",
			tape: []Value{frameOpen, frameOpen, one, closeP, one}, pointer: 4, want: true,
			why: "the inner frame matched; the outer one is still open (the recursion shape)",
		},
		{
			name: "after a closed USER paren", tape: []Value{userOpen, one, closeP, one}, pointer: 3, want: false,
			why: "a balanced user group leaves the pointer at top level",
		},
		{
			name: "frame open AFTER the pointer", tape: []Value{one, frameOpen}, pointer: 0, want: false,
			why: "only unmatched opens BEFORE the pointer enclose it",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// sawFnFrame is the monotonic latch real execution sets at the
			// splice site; set it here so these cases exercise the SCAN
			// rather than the fast path, which the sibling test pins.
			e := &Engine{Tape: NewTape(tc.tape, 4), Pointer: tc.pointer, sawFnFrame: true}
			if got := e.inFnFrame(); got != tc.want {
				t.Errorf("inFnFrame = %v, want %v — %s", got, tc.want, tc.why)
			}
		})
	}
}

// The fast path: with no frame ever spliced there can be no unmatched
// frame-open, so the prefix scan is skipped. Without it, a flat program pays a
// full walk per literal and Compile is quadratic in program length for a
// program containing no functions at all (PR #378 review, P3).
//
// The latch is what makes this safe, and its soundness argument is narrow
// enough to pin: inFnFrame is only ever reached with a recorder installed
// (every call site short-circuits on `e.recorder != nil` first), and both
// frame-splice sites set the latch under that same guard — so a frame cannot
// reach the tape unlatched while anything is looking. The end-to-end proof
// that the latch really is set lives in TestFnValueApplicationIsRecordedUnnamed
// and lang/go's round-trip suite, which would suppress nothing without it.
func TestInFnFrameFastPath(t *testing.T) {
	tape := []Value{NewFrameOpen(&FnFrameMeta{}), NewInteger(1)}

	unlatched := &Engine{Tape: NewTape(tape, 4), Pointer: 1}
	if unlatched.inFnFrame() {
		t.Error("unlatched engine scanned the tape — the fast path is not firing")
	}

	latched := &Engine{Tape: NewTape(tape, 4), Pointer: 1, sawFnFrame: true}
	if !latched.inFnFrame() {
		t.Error("latched engine missed a frame that IS on the tape")
	}
}

// BenchmarkFlatProgramRecording measures the shape the fast path exists for: a
// long, function-free program with a recorder installed. Run with -benchtime
// to vary N; the point is that cost stays linear rather than quadratic.
func BenchmarkFlatProgramRecording(b *testing.B) {
	prog := make([]Value, 0, 2000)
	for i := 0; i < 2000; i++ {
		prog = append(prog, NewInteger(int64(i)))
	}
	r, err := NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := NewTop(r)
		e.SetRecorder(&frameRec{})
		if _, err := e.Run(prog); err != nil {
			b.Fatal(err)
		}
	}
}

// recordDispatch picks the `returns` count the recorder's skip accounting
// needs, and the three arms answer differently for a reason each.
func TestRecordDispatch(t *testing.T) {
	frameOpen := NewFrameOpen(&FnFrameMeta{})

	t.Run("a native reports its real return count", func(t *testing.T) {
		rec := &frameRec{}
		e := &Engine{Tape: NewTape([]Value{NewInteger(1)}, 4)}
		e.SetRecorder(rec)
		e.recordDispatch("add", 2, []Value{NewInteger(3)})
		if len(rec.calls) != 1 || rec.calls[0].returns != 1 {
			t.Fatalf("calls = %+v, want one call with returns=1 (the value it produced)", rec.calls)
		}
	})

	t.Run("a frame splice reports zero", func(t *testing.T) {
		// The spliced skeleton is not a return count — passing its length is
		// the over-count that swallowed later literals. The frame's collapse
		// accounts for its own survivors via the RecorderSkipper hook.
		rec := &frameRec{}
		e := &Engine{Tape: NewTape([]Value{NewInteger(1)}, 4)}
		e.SetRecorder(rec)
		e.recordDispatch("inc", 1, []Value{frameOpen, NewInteger(1), NewCloseParen()})
		if len(rec.calls) != 1 || rec.calls[0].returns != 0 {
			t.Fatalf("calls = %+v, want one call with returns=0 (the skeleton is not a return count)", rec.calls)
		}
	})

	t.Run("a dispatch inside a frame is suppressed", func(t *testing.T) {
		// The callee's own internals: the enclosing Call already stands for
		// them, and recording them leaked body fragments into the caller's form.
		rec := &frameRec{}
		e := &Engine{Tape: NewTape([]Value{frameOpen, NewInteger(1)}, 4), Pointer: 1,
			sawFnFrame: true} // latched at the splice in real execution
		e.SetRecorder(rec)
		e.recordDispatch("add", 2, []Value{NewInteger(3)})
		if len(rec.calls) != 0 {
			t.Errorf("calls = %+v, want none — a body dispatch is not the caller's program", rec.calls)
		}
	})
}

// A function VALUE applied off the stack reaches the tape through
// execFnDefSig, which bypasses execMatch entirely. Without an event there it
// recorded NOTHING for the application, and the form silently replayed to the
// function instead of its result.
//
// It reports an EMPTY name deliberately, even for a named value: a
// `Call{Name, Arity}` re-invokes by name and does not consume a receiver,
// while an application consumes the fn value the stack already holds, so a
// named Call would strand it. stackform.Replayable declines the empty name
// rather than replaying a lie (NUR077).
func TestFnValueApplicationIsRecordedUnnamed(t *testing.T) {
	for _, tc := range []struct {
		name string
		fn   Value
	}{
		{"anonymous", anonFnVal(
			[]FnParam{{Name: "n", Type: TInteger}}, []*Type{TInteger},
			parenBody(NewWord("cadd"), NewWord("n"), NewWord("n")))},
		{"named", namedFnVal("dbl",
			[]FnParam{{Name: "n", Type: TInteger}}, []*Type{TInteger},
			parenBody(NewWord("cadd"), NewWord("n"), NewWord("n")))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := covRegistry(t, nil)
			rec := &frameRec{}
			e := NewTop(r)
			e.SetRecorder(rec)
			if _, err := e.Run([]Value{tc.fn, NewInteger(4)}); err != nil {
				t.Fatalf("run: %v", err)
			}
			var unnamed int
			for _, c := range rec.calls {
				if c.name == "" {
					unnamed++
				}
			}
			if unnamed != 1 {
				t.Errorf("recorded calls = %+v, want exactly one with an empty name "+
					"(the unreplayable application); none means the form silently drops it",
					rec.calls)
			}
		})
	}
}
