package core

import (
	"errors"
	"testing"
)

var errNoLens = errors.New("no lens")

// stubLensRuntime is a CompiledRuntime that records what the lens cache asks of
// it and answers on command. core/go is gated by its OWN suite (ADR-008, the
// same reason invoke_fnhome_test.go drives InvokeCallbackFn directly), and the
// VM piece is not linked here — so the seam is stubbed rather than left
// "covered from above", which would leave both branches reading as dead code.
type stubLensRuntime struct {
	stamps int
	land   bool // StampDetached lands a ref on the body's *BoruImpl
	ran    bool // InvokeCompiled claims the call
	res    []Value
	err    error
	args   []Value
}

func (s *stubLensRuntime) StampDetached(_ *Registry, fd FnDefInfo, _ SrcPos) {
	s.stamps++
	if !s.land {
		return
	}
	if bi, ok := fd.Signatures[0].Impl.(*BoruImpl); ok {
		bi.Compiled = "a ref (opaque to core)"
	}
}

func (s *stubLensRuntime) InvokeCompiled(_ *Registry, _ *Signature, args []Value) ([]Value, error, bool) {
	s.args = args
	return s.res, s.err, s.ran
}

func (s *stubLensRuntime) ClosureAsFnDef(_ *Registry, v Value) (Value, bool) { return v, false }

func withStubLensRuntime(t *testing.T, rt CompiledRuntime) {
	t.Helper()
	prev := InstallCompiledRuntime(rt)
	t.Cleanup(func() { InstallCompiledRuntime(prev) })
}

func lensSegs() []ReachSeg { return []ReachSeg{{KeyLit: NewWord("a")}} }

// compiledLensSig declines without asking the runtime anything at all when
// there is nothing to ask FOR: no cache to write into, no registry, or a
// registry where runtime stamping was never armed. The last is the one that
// matters — unarmed is the interpreter mode, where interpreting the chain is
// the right answer, and stamping there would break the mode contract every
// other stamp site holds.
func TestCompiledLensSigDeclinesWithoutArming(t *testing.T) {
	rt := &stubLensRuntime{land: true}
	withStubLensRuntime(t, rt)

	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	for _, c := range []struct {
		name string
		reg  *Registry
		lu   *lensUnit
	}{
		{"no cache", r, nil},
		{"no registry", nil, &lensUnit{}},
		{"unarmed registry", r, &lensUnit{}},
	} {
		if got := compiledLensSig(c.reg, c.lu, lensSegs()); got != nil {
			t.Errorf("%s: expected no signature, got %v", c.name, got)
		}
	}
	if rt.stamps != 0 {
		t.Errorf("nothing should have been stamped, got %d attempts", rt.stamps)
	}
}

// An armed registry stamps ONCE and reuses the result — the whole point of
// hanging the cache on the payload, since a lens in `each $.name people`
// applies per element.
func TestCompiledLensSigStampsOnceAndCaches(t *testing.T) {
	rt := &stubLensRuntime{land: true}
	withStubLensRuntime(t, rt)

	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.EnableRuntimeStamping()

	lu := &lensUnit{}
	first := compiledLensSig(r, lu, lensSegs())
	if first == nil {
		t.Fatal("an armed registry with a landing stamp must yield a signature")
	}
	if got := compiledLensSig(r, lu, lensSegs()); got != first {
		t.Errorf("second call returned %v, want the cached %v", got, first)
	}
	if rt.stamps != 1 {
		t.Errorf("stamped %d times, want exactly 1", rt.stamps)
	}
	// The body is the lens chain over the receiver PARAMETER — not a
	// hand-lowered second model of what a segment means.
	if n := len(first.Params); n != 1 || first.Params[0].Name != lensParam {
		t.Errorf("params = %v, want one %q", first.Params, lensParam)
	}
}

// A DECLINED stamp is cached too: declining is a property of the body, not of
// the moment, so re-paying the compile on every application would be pure loss.
func TestCompiledLensSigCachesTheDecline(t *testing.T) {
	rt := &stubLensRuntime{land: false}
	withStubLensRuntime(t, rt)

	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.EnableRuntimeStamping()

	lu := &lensUnit{}
	for i := 0; i < 3; i++ {
		if got := compiledLensSig(r, lu, lensSegs()); got != nil {
			t.Fatalf("call %d: a declined stamp must yield no signature, got %v", i, got)
		}
	}
	if rt.stamps != 1 {
		t.Errorf("stamped %d times, want exactly 1 — the decline must be cached", rt.stamps)
	}
}

// NewReach allocates the shared cache, and leaves one already present alone —
// the propagation fn_params.go depends on when it rebuilds a ReachInfo around
// the same lens.
func TestNewReachCarriesTheLensCache(t *testing.T) {
	fresh, err := AsReach(NewReach(ReachInfo{Segments: lensSegs()}))
	if err != nil {
		t.Fatalf("AsReach: %v", err)
	}
	if fresh.unit == nil {
		t.Error("a constructed reach must carry a cache to stamp into")
	}

	existing := &lensUnit{}
	carried, err := AsReach(NewReach(ReachInfo{Segments: lensSegs(), unit: existing}))
	if err != nil {
		t.Fatalf("AsReach: %v", err)
	}
	if carried.unit != existing {
		t.Error("an existing cache must ride through, or a rebuilt ReachInfo re-stamps per call")
	}
}

// lastReachResult is shared by both lanes so they cannot drift on what "the
// value of a lens" means.
func TestLastReachResult(t *testing.T) {
	if _, err := lastReachResult([]Value{NewInteger(1)}, errNoLens); err != errNoLens {
		t.Errorf("an error must win, got %v", err)
	}
	if _, err := lastReachResult(nil, nil); err == nil {
		t.Error("an empty result must report that the lens produced no value")
	}
	got, err := lastReachResult([]Value{NewInteger(1), NewInteger(9)}, nil)
	if err != nil {
		t.Fatalf("lastReachResult: %v", err)
	}
	if s := got.String(); s != "9" {
		t.Errorf("the read is the chain's LAST value, got %s", s)
	}
}

// ApplyReach takes the compiled lane when the runtime claims the call, and
// falls back to the interpreted chain when it declines — including the
// no-effect bail, which is exactly what ran=false means.
func TestApplyReachLanes(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.EnableRuntimeStamping()

	recv := NewInteger(7)
	segs := lensSegs()
	info, err := AsReach(NewReach(ReachInfo{Segments: segs}))
	if err != nil {
		t.Fatalf("AsReach: %v", err)
	}

	rt := &stubLensRuntime{land: true, ran: true, res: []Value{NewInteger(42)}}
	withStubLensRuntime(t, rt)
	got, err := ApplyReach(r, info, recv)
	if err != nil {
		t.Fatalf("ApplyReach (compiled lane): %v", err)
	}
	if got.String() != "42" {
		t.Errorf("compiled lane = %s, want 42", got.String())
	}
	if len(rt.args) != 1 || rt.args[0].String() != "7" {
		t.Errorf("the receiver must be the unit's one argument, got %v", rt.args)
	}

	// ran=false: the unit declined or bailed with no observable effect, and the
	// interpreted chain owns the answer. `7.a` is not a field read, so this
	// errors — which is the point: it proves the fallback actually ran rather
	// than the stub answering.
	declining, err := AsReach(NewReach(ReachInfo{Segments: segs}))
	if err != nil {
		t.Fatalf("AsReach: %v", err)
	}
	withStubLensRuntime(t, &stubLensRuntime{land: true, ran: false})
	if _, err := ApplyReach(r, declining, recv); err == nil {
		t.Error("expected the interpreted chain to run and reject `7.a`")
	}
}

// A stamped unit that RAN and DEFERRED is a designed bail, not an island: the
// runtime swallowed its internal error for the interpreter to resolve, and the
// interpreted chain that follows is that defer's replay. It must carry the
// attribution RunCompiled's top-level runtime-bail arm uses, or the ONE event
// is counted twice — once in the bail census, again in the interp-entry census.
//
// The nil-error decline is pinned alongside it, and that pairing is the whole
// contract: a lane that never reached the VM must stay UNATTRIBUTED, because
// attributing it would make the frontier's no-unattributed-entry assertions
// pass vacuously.
func TestApplyReachBailReplayAttribution(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.EnableRuntimeStamping()
	recv := NewInteger(7)

	lane := func(declined error) []string {
		t.Helper()
		info, aErr := AsReach(NewReach(ReachInfo{Segments: lensSegs()}))
		if aErr != nil {
			t.Fatalf("AsReach: %v", aErr)
		}
		withStubLensRuntime(t, &stubLensRuntime{land: true, ran: false, err: declined})
		var atts []string
		disarm := r.ArmInterpEntryHook(func(ev InterpEntry) {
			if !ev.CheckMode {
				atts = append(atts, ev.Attribution)
			}
		})
		// `7.a` is not a field read: the error proves the interpreted chain
		// really ran rather than the stub answering for it.
		if _, rErr := ApplyReach(r, info, recv); rErr == nil {
			t.Error("expected the interpreted chain to run and reject `7.a`")
		}
		disarm()
		if len(atts) == 0 {
			t.Fatal("the interpreted chain recorded no entry — the lane changed shape")
		}
		return atts
	}

	for _, att := range lane(errNoLens) {
		if att != "fallback:runtime-bail" {
			t.Errorf("a deferred unit's replay attributed %q, want fallback:runtime-bail", att)
		}
	}
	for _, att := range lane(nil) {
		if att != "" {
			t.Errorf("a unit that never ran attributed %q, want unattributed", att)
		}
	}
}
