package basic

import "testing"

// The `__pa` word's own pop. Nothing emits the word since the frame's
// DefCleanup marker pops the args list itself (core.stepDefCleanup); it
// stays registered as the word form of the same operation, so it is driven
// directly: the frame's args list and its capture baseline pop together,
// and with no Args stack at all the pop raises.
func TestPopArgsHandlerPopsTheFrameArgs(t *testing.T) {
	r := newTestRegistry(t)
	if err := r.Args.Push(NewList([]Value{NewInteger(7)})); err != nil {
		t.Fatal(err)
	}
	r.PushFnBaseline(map[string]int{})
	out, err := PopArgsHandler(nil, nil, nil, r)
	if err != nil || out != nil {
		t.Fatalf("__pa over a live frame: %v / %v, want nothing", out, err)
	}
	if d := r.Args.Depth(); d != 0 {
		t.Fatalf("args depth after __pa = %d, want 0", d)
	}
	if n := len(r.FnBaselines); n != 0 {
		t.Fatalf("the capture baseline must pop with the args list, %d left", n)
	}
	r.Args = nil
	if _, err := PopArgsHandler(nil, nil, nil, r); err == nil {
		t.Fatal("__pa with no Args stack must raise")
	}
}
