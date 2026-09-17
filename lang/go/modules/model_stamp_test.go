package modules

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"

	"github.com/boru-lang/boru/lang/go/native"
)

// Model action stamping (design/legacy/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.ignore
// Phase 6): buildActions stamps every action fn at model build
// (stampActionFn), so makeAction's InvokeCallback runs the body on the VM.
// Paired here: an anonymous spec lambda stamps under its ACTION name and a
// named fn keeps its own (positive); a capturing action DECLINES the stamp —
// recorded under the same name — and interprets with its capture intact
// (negative); and the user's spec value stays plain either way (StampFnValue
// clones — the stamp lives only on the model's private copy).

// modelStampEvent finds the stamp attempt with the given name, failing loudly
// when no attempt was recorded at all.
func modelStampEvent(t *testing.T, r *native.Registry, name string) core.StampEvent {
	t.Helper()
	for _, ev := range r.StampEvents() {
		if ev.Name == name {
			return ev
		}
	}
	t.Fatalf("no stamp attempt named %q recorded; events: %v", name, r.StampEvents())
	return core.StampEvent{}
}

func TestModelActionStampsAtBuild(t *testing.T) {
	r, _ := mw4Reg(t)
	r.EnableRuntimeStamping()
	if !mw4OkFlag(t, r, mw4Imp+
		`def myfn fn [[mod:Any] [Boolean] [true]]
		 def spec {src:'a: 1', actions:{gen:([mod:Any] => [true]), named: myfn/v}}
		 def m (Model.new spec) (Model.run m) get 'ok'`) {
		t.Fatal("stamped-actions run ok = false, want true")
	}
	if ev := modelStampEvent(t, r, "gen"); !ev.Stamped {
		t.Errorf("anonymous action must stamp under its action name; declined: %s", ev.Reason)
	}
	if ev := modelStampEvent(t, r, "myfn"); !ev.Stamped {
		t.Errorf("named action fn must stamp under its OWN name; declined: %s", ev.Reason)
	}

	// Clone semantics: the fn values inside the user's spec map must stay
	// plain — mutating a published value's shared impl from the build would
	// race concurrent readers.
	specV, ok := r.Defs.Top("spec")
	if !ok {
		t.Fatal("spec binding missing after the run")
	}
	sm, err := native.AsMap(specV)
	if err != nil {
		t.Fatalf("spec is not a map: %v", err)
	}
	actionsV, _ := sm.Get("actions")
	am, err := native.AsMap(actionsV)
	if err != nil {
		t.Fatalf("actions is not a map: %v", err)
	}
	for _, key := range []string{"gen", "named"} {
		fnV, _ := am.Get(key)
		fd, isFn := fnV.Data.(native.FnDefInfo)
		if !isFn {
			t.Fatalf("spec action %q is not a fn value: %v", key, fnV)
		}
		for i := range fd.Signatures {
			if compiler.CompiledRef(&fd.Signatures[i]) != nil {
				t.Errorf("spec action %q carries a compiled ref — the stamp must live on the model's private clone only", key)
			}
		}
	}
}

func TestModelActionCaptureDeclinesAndInterprets(t *testing.T) {
	r, _ := mw4Reg(t)
	r.EnableRuntimeStamping()
	// act is constructed in WORD context inside mk's body, so it CAPTURES the
	// enclosing param flag. Post the capture-slot landing (plan Phase 6.3)
	// captures per se stamp; THIS capture is a runtime-minted param value
	// with no compile identity, which used to decline on the identity gate —
	// since the §7a landing (2026-07-16) the detached compile mints an
	// identity on a clone of the frozen captured slice, so the action STAMPS
	// and the VM run binds the capture from the ref: the captured false must
	// flip the build's ok flag with NO action error, exactly as the
	// interpreter ran it before the landing.
	out := mcovRun(t, r, mw4Imp+
		`def mk fn [[flag:Boolean] [Map] [ def act ([mod:Any] => [flag]) {src:'a: 1', actions:{gen: act/v}} ]]
		 def m (Model.new (mk false)) def res (Model.run m) (res get 'ok') ((res get 'errs') size)`)
	if n, err := out[len(out)-1].AsConcreteInteger(); err != nil || n != 0 {
		t.Fatalf("capturing action errs size = %v (%v), want 0 (the capture must reach the body)", out[len(out)-1], err)
	}
	if ok, err := out[len(out)-2].AsConcreteBoolean(); err != nil || ok {
		t.Fatalf("captured-flag action ok = %v (%v), want false", out[len(out)-2], err)
	}
	ev := modelStampEvent(t, r, "act")
	if !ev.Stamped {
		t.Errorf("a capturing action must stamp post-§7a, got reason %q", ev.Reason)
	}
}

func TestModelActionDataContextLambdaDeclineIsRenamed(t *testing.T) {
	r, _ := mw4Reg(t)
	r.EnableRuntimeStamping()
	// A lambda built in DATA context (a map value) inside an afn's transparent
	// single-literal body runs no capture analysis —
	// pre-existing interpreter behavior: the body's read of the enclosing
	// param fails at ACTION time with undefined_word, surfacing in errs. The
	// stamp probe declines on the unresolvable read; the decline is recorded
	// under the ACTION name (the rename applies whether or not the stamp
	// lands), and stamping must not change the outcome.
	out := mcovRun(t, r, mw4Imp+
		`def mk ([flag:Boolean] => [{src:'a: 1', actions:{gen:([mod:Any] => [flag])}}])
		 def m (Model.new (mk false)) def res (Model.run m) (res get 'ok') ((res get 'errs') get 0)`)
	if s, err := out[len(out)-1].AsConcreteString(); err != nil || !strings.Contains(s, "undefined word: flag") {
		t.Fatalf("errs[0] = %v (%v), want the pre-existing undefined_word action error", out[len(out)-1], err)
	}
	if ok, err := out[len(out)-2].AsConcreteBoolean(); err != nil || ok {
		t.Fatalf("data-context action ok = %v (%v), want false", out[len(out)-2], err)
	}
	ev := modelStampEvent(t, r, "gen")
	if ev.Stamped {
		t.Error("the unresolvable body must decline the stamp")
	}
	if !strings.Contains(ev.Reason, "provenance") {
		t.Errorf("decline reason = %q, want the unknown-provenance probe decline", ev.Reason)
	}
}
