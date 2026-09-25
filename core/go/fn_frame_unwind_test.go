package core

import (
	"strings"
	"testing"
)

// Direct kernel pins for unwindLiveFrames / unwindFrameTail (fn_frame.go):
// a flow-control rewrite discarding a region must replay the canonical
// cleanup tail of every frame still OPEN in it — __DC truncation, the
// __pa Args/FnBaseline pop, and the force-forward undef pairs — and must
// NOT touch completed frames, user-written undef tokens, or nested
// content below the frame's own paren depth. The language-level twins
// (break/continue through live frames) live in
// lang/go/native/args_inert_test.go; these tests pin the tape mechanics
// over hand-built frames, kernel-only.

// unwindFixture builds an engine whose tape holds one spliced frame in
// the canonical shape, with the frame's per-call state (args list,
// baseline, one param binding, one body-local def) LIVE on the registry,
// and the pointer parked inside the frame body.
func unwindFixture(t *testing.T) (*Engine, *Registry) {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := New(r)

	// Simulate frame entry exactly as buildFnBodyHandler performs it.
	r.PushFnBaseline(r.Defs.Snapshot())
	if err := r.Args.Push(NewList([]Value{NewInteger(7)})); err != nil {
		t.Fatal(err)
	}
	InstallFrameBinding(r, "uwx", NewInteger(7))
	snap := r.Defs.Snapshot() // after param install: __DC truncates body-locals only
	r.Defs.Push("uwlocal", NewInteger(1))

	tokens := []Value{
		NewFrameOpenSpan(fnValueFrameMeta, 1),
		NewInteger(7), // the resolved unnamed arg
		NewWord("uwbody"),
	}
	tokens = AppendFrameTail(tokens, FrameTailSpec{
		Registry: r,
		Snapshot: snap,
		Names:    []string{"uwx"},
		Returns:  []*Type{TInteger},
		FuncName: "uw",
	})
	tokens = append(tokens, NewCloseParen())
	e.Tape = NewTape(tokens, 4)
	e.Pointer = 2 // parked at the body word: frame open stepped, tail not reached
	return e, r
}

func TestUnwindLiveFrameReplaysCleanupTail(t *testing.T) {
	e, r := unwindFixture(t)
	e.unwindLiveFrames(0, e.Tape.Len())

	if _, ok, _ := r.Args.Top(); ok {
		t.Error("per-call args list not popped by the unwind")
	}
	if r.Defs.Has("uwx") {
		t.Error("param binding not undef'd by the unwind")
	}
	if r.Defs.Has("uwlocal") {
		t.Error("body-local def not truncated by the unwind (__DC)")
	}
}

func TestUnwindSkipsCompletedFrame(t *testing.T) {
	// A frame whose close paren sits BELOW the pointer already ran its
	// tail; the unwind must not double-pop. Sentinel args entry stands in
	// for the enclosing call's state.
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := New(r)
	if err := r.Args.Push(NewList([]Value{NewInteger(1)})); err != nil {
		t.Fatal(err)
	}
	tokens := []Value{
		NewFrameOpenSpan(fnValueFrameMeta, 0),
		NewInteger(9),
		NewCloseParen(),
		NewInteger(9), // residual past the completed frame
	}
	e.Tape = NewTape(tokens, 4)
	e.Pointer = 3 // past the close: the frame is NOT live
	e.unwindLiveFrames(0, e.Tape.Len())
	if _, ok, _ := r.Args.Top(); !ok {
		t.Error("unwind popped state belonging to a COMPLETED frame")
	}
}

func TestUnwindNestedFramesPopLIFO(t *testing.T) {
	// Two live frames, inner inside outer: both tails replay, and the
	// depth gate keeps each tail owned by its own frame (the inner
	// frame's markers are depth 2 relative to the outer open).
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := New(r)

	// Outer frame entry.
	r.PushFnBaseline(r.Defs.Snapshot())
	if err := r.Args.Push(NewList([]Value{NewInteger(1)})); err != nil {
		t.Fatal(err)
	}
	InstallFrameBinding(r, "uwout", NewInteger(1))
	outerSnap := r.Defs.Snapshot()
	// Inner frame entry.
	r.PushFnBaseline(r.Defs.Snapshot())
	if err := r.Args.Push(NewList([]Value{NewInteger(2)})); err != nil {
		t.Fatal(err)
	}
	InstallFrameBinding(r, "uwin", NewInteger(2))
	innerSnap := r.Defs.Snapshot()

	inner := []Value{NewFrameOpenSpan(fnValueFrameMeta, 0), NewWord("uwibody")}
	inner = AppendFrameTail(inner, FrameTailSpec{
		Registry: r, Snapshot: innerSnap, Names: []string{"uwin"}, FuncName: "uwi",
	})
	inner = append(inner, NewCloseParen())

	tokens := []Value{NewFrameOpenSpan(fnValueFrameMeta, 0)}
	tokens = append(tokens, inner...)
	tokens = AppendFrameTail(tokens, FrameTailSpec{
		Registry: r, Snapshot: outerSnap, Names: []string{"uwout"}, FuncName: "uwo",
	})
	tokens = append(tokens, NewCloseParen())
	e.Tape = NewTape(tokens, 4)
	e.Pointer = 2 // inside the INNER frame's body: both frames live

	e.unwindLiveFrames(0, e.Tape.Len())
	if _, ok, _ := r.Args.Top(); ok {
		t.Error("nested unwind left a per-call args entry")
	}
	if r.Defs.Has("uwin") || r.Defs.Has("uwout") {
		t.Error("nested unwind left a param binding installed")
	}
}

func TestUnwindIgnoresUserUndefTokens(t *testing.T) {
	// A user-written `undef name` in the (skipped) body region carries no
	// ForceForward flag; the unwind must not execute it — only the
	// machine-generated tail pairs tear bindings down.
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := New(r)
	r.PushFnBaseline(r.Defs.Snapshot())
	if err := r.Args.Push(NewList(nil)); err != nil {
		t.Fatal(err)
	}
	r.Defs.Push("uwkeep", NewInteger(3))
	snap := r.Defs.Snapshot()

	tokens := []Value{
		NewFrameOpenSpan(fnValueFrameMeta, 0),
		NewWord("undef"), NewWord("uwkeep"), // USER tokens, not the tail's pairs
	}
	tokens = AppendFrameTail(tokens, FrameTailSpec{
		Registry: r, Snapshot: snap, Names: nil, FuncName: "uwu",
	})
	tokens = append(tokens, NewCloseParen())
	e.Tape = NewTape(tokens, 4)
	e.Pointer = 1

	e.unwindLiveFrames(0, e.Tape.Len())
	if !r.Defs.Has("uwkeep") {
		t.Error("unwind executed a USER undef token from the skipped body")
	}
	if _, ok, _ := r.Args.Top(); ok {
		t.Error("frame tail __pa not replayed")
	}
}

func TestUnwindClampsRegionAndHandlesOpenEndedFrame(t *testing.T) {
	// A frame whose close paren lies outside the discarded region (the
	// loop rewrite cuts mid-frame) is still live and still unwinds; the
	// region end is clamped to the tape.
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := New(r)
	r.PushFnBaseline(r.Defs.Snapshot())
	if err := r.Args.Push(NewList(nil)); err != nil {
		t.Fatal(err)
	}
	tokens := []Value{
		NewFrameOpenSpan(fnValueFrameMeta, 0),
		NewWord("uwbody"),
	}
	tokens = AppendFrameTail(tokens, FrameTailSpec{
		Registry: r, Snapshot: r.Defs.Snapshot(), Names: nil, FuncName: "uwc",
	})
	// Deliberately NO close paren: the frame extends past the region.
	e.Tape = NewTape(tokens, 4)
	e.Pointer = 1

	e.unwindLiveFrames(0, e.Tape.Len()+10) // over-long region clamps
	if _, ok, _ := r.Args.Top(); ok {
		t.Error("open-ended live frame's __pa not replayed")
	}
}

// runErrorFrame builds a spliced frame in the canonical shape with its
// per-call state LIVE on r — exactly as buildFnBodyHandler installs it at
// dispatch — around the given body, beneath a CALLER binding of the
// param's own name, so a replay of the tail is measurable twice over:
// once tears the frame down, twice pops the caller's binding as well.
func runErrorFrame(t *testing.T, r *Registry, body ...Value) []Value {
	t.Helper()
	InstallFrameBinding(r, "uwx", NewInteger(1)) // the caller's own uwx
	r.PushFnBaseline(r.Defs.Snapshot())
	if err := r.Args.Push(NewList([]Value{NewInteger(7)})); err != nil {
		t.Fatal(err)
	}
	InstallFrameBinding(r, "uwx", NewInteger(7)) // the frame's param
	snap := r.Defs.Snapshot()
	r.Defs.Push("uwlocal", NewInteger(1)) // a body-local def
	tokens := []Value{NewFrameOpenSpan(fnValueFrameMeta, 1), NewInteger(7)}
	tokens = append(tokens, body...)
	tokens = AppendFrameTail(tokens, FrameTailSpec{
		Registry:     r,
		Snapshot:     snap,
		Names:        []string{"uwx"},
		Returns:      []*Type{TInteger},
		FuncName:     "uw",
		EvalResidual: true,
	})
	return append(tokens, NewCloseParen())
}

// requireFrameTornDownOnce asserts the frame runErrorFrame built is gone
// from the registry — the tail replayed — and the caller's binding beneath
// it is intact: the tail replayed exactly ONCE.
func requireFrameTornDownOnce(t *testing.T, r *Registry) {
	t.Helper()
	if d := r.Args.Depth(); d != 0 {
		t.Errorf("per-call args list not popped by the error unwind: depth %d", d)
	}
	if len(r.FnBaselines) != 0 {
		t.Errorf("fn baseline not popped by the error unwind: %d left", len(r.FnBaselines))
	}
	if r.Defs.Has("uwlocal") {
		t.Error("body-local def not truncated by the error unwind (__DC)")
	}
	v, ok := r.Defs.Top("uwx")
	if !ok {
		t.Fatal("the caller's own binding was popped: the frame tail replayed twice")
	}
	if n, _ := AsInteger(v); n != 1 {
		t.Errorf("the frame's param still shadows the caller's binding: uwx = %v", v)
	}
}

// TestRunErrorUnwindsLiveFrame pins the spliced frame's error-path
// contract (faultReturn, NUR201): an error raised inside a frame's body
// abandons the tape, and the frame still open on it is torn down as its
// cleanup tail would have — the body-local defs truncated, the per-call
// Args list and FnBaseline popped, the params undef'd — so a `do` trapping
// the error upstream resumes with none of the callee's state leaked into
// the caller's scope (`def t 0  def g fn [[][Integer][def t 9 raise 'x']]
// do [g]  t` read 9 on the interpreter for the compiled lane's 0).
func TestRunErrorUnwindsLiveFrame(t *testing.T) {
	r := covRegistry(t, registerCraise)
	e := NewTop(r)
	_, err := e.Run(runErrorFrame(t, r, NewWord("craise")))
	if err == nil || !strings.Contains(err.Error(), "craise boom") {
		t.Fatalf("want the body's error, got %v", err)
	}
	requireFrameTornDownOnce(t, r)
}

// TestRunErrorUnwindsFrameOnceAfterResidualError pins the other frame
// error site: the in-frame residual evaluation at the DefCleanup marker
// (a computing body's pending container) raising. The marker no longer
// replays the tail itself — the frame is still open on the tape, so the
// run's fault return replays it — and the tail runs exactly once: a
// second replay would pop the CALLER's args entry and its same-named
// binding.
func TestRunErrorUnwindsFrameOnceAfterResidualError(t *testing.T) {
	r := runReg(t)
	e := NewTop(r)
	_, err := e.Run(runErrorFrame(t, r, pendingList(NewWord("cfail"), NewInteger(1))))
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want the residual's error, got %v", err)
	}
	requireFrameTornDownOnce(t, r)
}
