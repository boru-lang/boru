package core

// Tests for the leaf-fn frame-skeleton memoization in buildFnBodyHandler
// (design/legacy/INTERPRETER-PYTHON-PARITY.10.ignore Phase A / F5-full): the constant
// token skeleton is built once per signature and copied per call with the
// arg cells patched, and the per-call args list is elided (a shared empty
// list) when the body provably never reads `args`.

import (
	"sync"
	"testing"
)

// skelHandler digs the dispatch handler out of an installed fn so the
// tests can invoke it directly and inspect the returned token stream.
func skelHandler(t *testing.T, r *Registry, name string) Handler {
	t.Helper()
	top, ok := r.Defs.Top(name)
	if !ok {
		t.Fatalf("%s not installed", name)
	}
	fd, ok := top.Data.(FnDefInfo)
	if !ok {
		t.Fatalf("%s is not an FnDef", name)
	}
	var sig *Signature
	for i := range fd.Signatures {
		if !fd.Signatures[i].Fallback {
			sig = &fd.Signatures[i]
			break
		}
	}
	if sig == nil {
		t.Fatalf("%s has no non-fallback sig", name)
	}
	h := sig.DispatchHandler()
	if h == nil {
		t.Fatalf("%s sig has no dispatch handler", name)
	}
	return h
}

// popFrameEntry unwinds the registry state one direct handler call pushed
// (baseline + args + the named-param/capture bindings), so a test can call
// the handler without executing the returned cleanup tail.
func popFrameEntry(t *testing.T, r *Registry, names ...string) {
	t.Helper()
	for _, n := range names {
		UninstallDef(r, n)
	}
	if err := PopFrameArgs(r); err != nil {
		t.Fatalf("PopFrameArgs: %v", err)
	}
}

// TestSkeletonPerCallCopyIsolated pins the mandatory-copy invariant: the
// handler's returned slice is a fresh copy of the memoized skeleton, so
// stampResultPos's in-place ReturnCheck mutation on one call's result can
// never leak into the next call (or, under ForkConcurrent, a sibling's).
func TestSkeletonPerCallCopyIsolated(t *testing.T) {
	r := covRegistry(t, nil)
	InstallFnDef(r, "leafrc", FnDefInfo{
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "n", Type: TInteger}},
			Returns:    []*Type{TInteger},
			Impl:       Boru([]Value{NewWord("n")}),
			BarrierPos: BarrierAllForward,
		}},
	})
	h := skelHandler(t, r, "leafrc")

	out1, err := h([]Value{NewInteger(1)}, nil, nil, r)
	if err != nil {
		t.Fatalf("call 1: %v", err)
	}
	popFrameEntry(t, r, "n")

	// Mutate call 1's result the way execMatch does at the call site.
	stampResultPos(out1, &SrcPos{Row: 41, Col: 7})
	stamped := false
	for _, v := range out1 {
		if IsReturnCheck(v) {
			rc, _ := AsReturnCheck(v)
			if rc.Pos.Row == 41 {
				stamped = true
			}
		}
	}
	if !stamped {
		t.Fatal("stampResultPos did not stamp call 1's ReturnCheck — test premise broken")
	}

	out2, err := h([]Value{NewInteger(2)}, nil, nil, r)
	if err != nil {
		t.Fatalf("call 2: %v", err)
	}
	popFrameEntry(t, r, "n")

	// Call 2's copy must carry the pristine zero Pos, not call 1's stamp.
	for _, v := range out2 {
		if IsReturnCheck(v) {
			rc, _ := AsReturnCheck(v)
			if rc.Pos.Row != 0 {
				t.Fatalf("call 2's ReturnCheck carries call 1's stamp (Row=%d) — skeleton shared, not copied", rc.Pos.Row)
			}
		}
	}
	// Negative: the two calls must not share a backing array at all.
	if len(out1) > 0 && len(out2) > 0 && &out1[0] == &out2[0] {
		t.Fatal("both calls returned the same backing array")
	}
}

// TestSkeletonInterleavedParams pins the unnamedIdx mapping: with named and
// unnamed params interleaved, the frame-head arg cells receive args[i] at
// the UNNAMED param positions (not args[:u]).
func TestSkeletonInterleavedParams(t *testing.T) {
	r := covRegistry(t, nil)
	InstallFnDef(r, "leafmix", FnDefInfo{
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "a", Type: TInteger}, {Type: TAny}, {Name: "b", Type: TInteger}},
			Returns:    []*Type{TAny},
			Impl:       Boru([]Value{NewWord("a")}),
			BarrierPos: BarrierAllForward,
		}},
	})
	h := skelHandler(t, r, "leafmix")

	args := []Value{NewInteger(10), NewString("mid"), NewInteger(30)}
	out, err := h(args, nil, nil, r)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	popFrameEntry(t, r, "a", "b")

	info, err := AsFrameOpen(out[0])
	if err != nil {
		t.Fatalf("out[0] is not a frame open: %v", err)
	}
	if info.ArgSpan != 1 {
		t.Fatalf("ArgSpan = %d, want 1 (one unnamed param)", info.ArgSpan)
	}
	// The single arg cell must hold args[1] ("mid"), the unnamed position —
	// not args[0] (the first arg overall).
	got, err := out[1].AsConcreteString()
	if err != nil || got != "mid" {
		t.Fatalf("arg cell = %s (err %v), want the unnamed-position arg \"mid\"", out[1].String(), err)
	}
}

// TestSkeletonArgsElision pins the args-list behavior of the fast path:
// a leaf body that never reads `args` pushes the shared empty list; a body
// that reads it (directly or through a word-macro splice) gets the real
// per-call copy.
func TestSkeletonArgsElision(t *testing.T) {
	install := func(r *Registry, name string, body []Value) {
		InstallFnDef(r, name, FnDefInfo{
			Signatures: []Signature{{
				Params:     []FnParam{{Name: "n", Type: TInteger}},
				Returns:    []*Type{TAny},
				Impl:       Boru(body),
				BarrierPos: BarrierAllForward,
			}},
		})
	}
	argsTopLen := func(t *testing.T, r *Registry) int {
		t.Helper()
		top, ok, err := r.Args.Top()
		if err != nil || !ok {
			t.Fatalf("no args entry pushed (ok=%v err=%v)", ok, err)
		}
		lst, err := AsList(top)
		if err != nil {
			t.Fatalf("args entry is not a concrete list: %v", err)
		}
		return lst.Len()
	}

	t.Run("elided when body never reads args", func(t *testing.T) {
		r := covRegistry(t, nil)
		install(r, "leafplain", []Value{NewWord("cadd"), NewWord("n"), NewWord("n")})
		h := skelHandler(t, r, "leafplain")
		if _, err := h([]Value{NewInteger(3)}, nil, nil, r); err != nil {
			t.Fatal(err)
		}
		if n := argsTopLen(t, r); n != 0 {
			t.Errorf("expected the shared empty args list, got %d entries", n)
		}
		popFrameEntry(t, r, "n")
	})

	t.Run("real list when body reads args", func(t *testing.T) {
		r := covRegistry(t, nil)
		install(r, "leafargs", []Value{NewWord("args")})
		h := skelHandler(t, r, "leafargs")
		if _, err := h([]Value{NewInteger(3)}, nil, nil, r); err != nil {
			t.Fatal(err)
		}
		if n := argsTopLen(t, r); n != 1 {
			t.Errorf("expected the real 1-entry args list, got %d entries", n)
		}
		popFrameEntry(t, r, "n")
	})

	t.Run("real list when args hides behind a word-macro", func(t *testing.T) {
		r := covRegistry(t, nil)
		InstallDef(r, "margs", NewSplice(NewList([]Value{NewWord("args")})))
		install(r, "leafmacro", []Value{NewWord("margs")})
		h := skelHandler(t, r, "leafmacro")
		if _, err := h([]Value{NewInteger(3)}, nil, nil, r); err != nil {
			t.Fatal(err)
		}
		if n := argsTopLen(t, r); n != 1 {
			t.Errorf("macro-hidden args read must keep the real list, got %d entries", n)
		}
		popFrameEntry(t, r, "n")
	})

	t.Run("elided when args appears only in quoted data", func(t *testing.T) {
		r := covRegistry(t, nil)
		quoted := NewList([]Value{NewWord("args")})
		quoted.Quoted = true
		install(r, "leafquoted", []Value{quoted})
		h := skelHandler(t, r, "leafquoted")
		if _, err := h([]Value{NewInteger(3)}, nil, nil, r); err != nil {
			t.Fatal(err)
		}
		if n := argsTopLen(t, r); n != 0 {
			t.Errorf("quoted data is not a read — expected elision, got %d entries", n)
		}
		popFrameEntry(t, r, "n")
	})
}

// TestBodyReferencesArgs unit-pins the scan itself, including the
// macro-splice recursion and its cycle guard.
func TestBodyReferencesArgs(t *testing.T) {
	r := covRegistry(t, nil)

	if !bodyReferencesArgs(r, []Value{NewWord("args")}) {
		t.Error("direct args read not detected")
	}
	if !bodyReferencesArgs(r, []Value{NewList([]Value{NewWord("args")})}) {
		t.Error("args inside a code list not detected")
	}
	if bodyReferencesArgs(r, []Value{NewWord("cadd"), NewInteger(1)}) {
		t.Error("false positive on an args-free body")
	}
	// Reach receiver (`args.0`).
	if !bodyReferencesArgs(r, []Value{NewReachFromKeys(NewWord("args"), []Value{NewInteger(0)})}) {
		t.Error("args as a Reach receiver not detected")
	}
	// Mutually-recursive macros, one of which expands to args.
	InstallDef(r, "sm1", NewSplice(NewList([]Value{NewWord("sm2")})))
	InstallDef(r, "sm2", NewSplice(NewList([]Value{NewWord("sm1"), NewWord("args")})))
	if !bodyReferencesArgs(r, []Value{NewWord("sm1")}) {
		t.Error("args through mutually-recursive macros not detected")
	}
	// Pure macro cycle without args: must terminate and report false.
	InstallDef(r, "cyc", NewSplice(NewList([]Value{NewWord("cyc")})))
	if bodyReferencesArgs(r, []Value{NewWord("cyc")}) {
		t.Error("macro cycle without args reported true")
	}
	// nil registry: no macro resolution, direct hits only.
	if bodyReferencesArgs(nil, []Value{NewWord("sm1")}) {
		t.Error("nil registry must not resolve macros")
	}
	if !bodyReferencesArgs(nil, []Value{NewWord("args")}) {
		t.Error("nil registry must still detect a direct read")
	}
}

// The three tests below are the SLOW-path (needsFrameState) twins of arms
// that used to be exercised by leaf fns before the fast path claimed those:
// the Args.Push error unwind, the list-param Quoted adjustment, and the
// unnamed-param FrameOpenSpan stamp. A `def` in the body forces the slow
// path (bodyNeedsFrameState).

func TestSlowPathArgsPushError(t *testing.T) {
	r := covRegistry(t, nil)
	s := FnSig{
		Params: []FnParam{{Name: "n", Type: TInteger}},
		Impl:   Boru([]Value{NewWord("def"), NewWord("loc"), NewWord("n")}),
	}
	h := buildFnBodyHandler(r, "slowerr", s, FnDefInfo{}, &FnFrameMeta{Name: "slowerr"})
	r.Args = nil // Push fails; the handler must unwind the baseline
	if _, err := h([]Value{NewInteger(1)}, nil, nil, r); err == nil {
		t.Error("expected an error from the nil args stack on the slow path")
	}
	if r.TopFnBaseline() != nil {
		t.Error("baseline not unwound after the slow-path Push failure")
	}
}

func TestSlowPathListParamQuoted(t *testing.T) {
	r := covRegistry(t, nil)
	s := FnSig{
		Params: []FnParam{{Name: "xs", Type: TList}},
		Impl:   Boru([]Value{NewWord("def"), NewWord("loc"), NewInteger(1), NewWord("xs")}),
	}
	h := buildFnBodyHandler(r, "slowlist", s, FnDefInfo{}, &FnFrameMeta{Name: "slowlist"})
	if _, err := h([]Value{NewList([]Value{NewInteger(7)})}, nil, nil, r); err != nil {
		t.Fatalf("slow-path call: %v", err)
	}
	bound, ok := r.Defs.Top("xs")
	if !ok {
		t.Fatal("list param not installed")
	}
	if !bound.Quoted {
		t.Error("slow path must install list params Quoted (data, not code)")
	}
	popFrameEntry(t, r, "xs")
}

func TestSlowPathUnnamedParamSpan(t *testing.T) {
	r := covRegistry(t, nil)
	s := FnSig{
		Params: []FnParam{{Type: TInteger}},
		Impl:   Boru([]Value{NewWord("def"), NewWord("loc"), NewInteger(1)}),
	}
	h := buildFnBodyHandler(r, "slowspan", s, FnDefInfo{}, &FnFrameMeta{Name: "slowspan"})
	out, err := h([]Value{NewInteger(9)}, nil, nil, r)
	if err != nil {
		t.Fatalf("slow-path call: %v", err)
	}
	info, err := AsFrameOpen(out[0])
	if err != nil || info.ArgSpan != 1 {
		t.Fatalf("slow path FrameOpenSpan: ArgSpan=%d err=%v, want 1", info.ArgSpan, err)
	}
	if n, err := AsInteger(out[1]); err != nil || n != 9 {
		t.Fatalf("slow path arg cell = %s, want 9", out[1].String())
	}
	popFrameEntry(t, r)
}

// TestSkeletonEndToEndRecursion runs the real dispatch path over the
// memoized skeleton: nested leaf-fn calls (cquad→ctwice→cadd) produce the
// same results on repeated runs, and the per-call registry state stays
// balanced.
func TestSkeletonEndToEndRecursion(t *testing.T) {
	r := covRegistry(t, nil)
	for i := 0; i < 3; i++ {
		out, err := NewTop(r).Run([]Value{NewWord("cquad"), NewInteger(3)})
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if len(out) != 1 {
			t.Fatalf("run %d: %d results, want 1", i, len(out))
		}
		if n, err := AsInteger(out[0]); err != nil || n != 12 {
			t.Fatalf("run %d: cquad 3 = %s, want 12", i, out[0].String())
		}
		if _, ok, _ := r.Args.Top(); ok {
			t.Fatalf("run %d: args stack not balanced after run", i)
		}
	}
}

// TestSkeletonForkConcurrent runs the same leaf fn from two concurrent
// forks — the memoized skeleton and the shared empty args list are shared
// read-only state, so this must be race-free (run under -race).
func TestSkeletonForkConcurrent(t *testing.T) {
	r := covRegistry(t, nil)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f := r.ForkConcurrent()
			for i := 0; i < 50; i++ {
				out, err := NewTop(f).Run([]Value{NewWord("ctwice"), NewInteger(5)})
				if err != nil {
					errs <- err
					return
				}
				if n, _ := AsInteger(out[0]); n != 10 {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent leaf call: %v", err)
	}
}
