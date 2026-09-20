package test

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

func asIntegerVal(v core.Value) (int64, error) { return core.AsInteger(v) }

// stampEventsForNet is stampEventsFor with the module resolver installed,
// for module bodies that import boru:net (the serve-raw storing shape).
func stampEventsForNet(t *testing.T, src string) []core.StampEvent {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	modules.InstallResolver(reg)
	reg.EnableRuntimeStamping()
	engine := native.NewTop(reg)
	vals, pErr := parser.Parse(src)
	if pErr != nil {
		t.Fatal(pErr)
	}
	if _, rErr := engine.Run(vals); rErr != nil {
		t.Fatal(rErr)
	}
	return reg.StampEvents()
}

// TestStampDynEnvLateArmDrift pins the dynEnv plan/lowering drift fix
// (mini-s3's s3-serve): a stored-fn unit whose body FIRST builds a service
// whose handler binds a COMPUTED def (`def old (if …)` — an evDynBind with
// an event source), and only LATER reaches a dyn-body dispatch
// (`do … error …`) that arms the program-wide dynEnv mode. Before the fix
// the handler's unit was planned BEFORE the arming (no dyn-bind source
// promotion) and lowered AFTER it (widened — every def needs its
// OpBindDynScope twin), so Finalize declined "dynamic-scope def `old` of
// unpromoted computed value" and the whole detached stamp died ("finalize
// left the unit unstamped"). The probes now carry their terminal dynEnv
// back into the real pass, so every unit plans under the widened mode.
func TestStampDynEnvLateArmDrift(t *testing.T) {
	evs := stampEventsForNet(t, `
module [
  import "boru:net"
  def mk-store fn [[] [Any] [
    def store (service {objects: {}})
    add {op:"append"} ([req:Map state:Any] => [
      def objects state.objects
      def k req.key
      def cur (objects get k)
      def old (if (cur eq None) [ "" ] [ cur ])
      def nb (old add req.chunk)
      objects set (k) nb
      drop
      size nb
    ]) store
    add {op:"size"} ([req:Map state:Any] => [
      def cur (state.objects get req.key)
      if (cur eq None) [ -1 ] [ size cur ]
    ]) store
    store
  ]]
  def risky fn [[b:Any store:Any] [Any] [
    def ok (do b error [ drop false ])
    if ok [ 1 ] [ 0 ]
  ]]
  def srv fn [[opts:Map] [Any] [
    def store (mk-store)
    risky [ call {op:"append" key: "k" chunk: "c"} store drop true ] store
  ]]
  def srv2 fn [[opts:Map] [Any] [
    def store (mk-store)
    Net.serve-raw {tcp: opts.port} ([conn:Any] => [ risky [ 0 ] store ])
  ]]
  def srv3 fn [[opts:Map] [Any] [
    spawn [ risky [ 0 ] 0 ]
  ]]
  export "DrfT" { mk: mk-store/v srv: srv/v srv2: srv2/v srv3: srv3/v }
] import
`)
	// The store factory and the do-bearing helper stamped before the fix
	// too — pinned so the drift fix cannot regress them.
	for _, name := range []string{"mk-store", "risky"} {
		if reason, ok := stampOutcome(evs, name); !ok {
			t.Errorf("%s did not stamp: %s", name, reason)
		}
	}
	// srv is the drift shape: handler unit planned before the do arms
	// dynEnv, lowered after. srv2 routes the arming through a CAPTURING
	// stored handler (tryReturnedClosure — mini-s3's serve-raw shape);
	// srv3 through a spawn body (compileStoredBody). Each probe must
	// carry its terminal dynEnv back to the real pass.
	for _, name := range []string{"srv", "srv2", "srv3"} {
		if reason, ok := stampOutcome(evs, name); !ok {
			t.Errorf("%s did not stamp: %s", name, reason)
		}
	}
}

// TestStampDynEnvDriftParity runs the drifted shape end-to-end: the stamped
// units must produce the interpreter's answer (the service handler binds and
// reads its computed def through the widened dyn-scope lowering).
func TestStampDynEnvDriftParity(t *testing.T) {
	out := runStampedModule(t, `
module [
  def mk-store fn [[] [Any] [
    def store (service {objects: {}})
    add {op:"append"} ([req:Map state:Any] => [
      def objects state.objects
      def k req.key
      def cur (objects get k)
      def old (if (cur eq None) [ "" ] [ cur ])
      def nb (old add req.chunk)
      objects set (k) nb
      drop
      size nb
    ]) store
    add {op:"size"} ([req:Map state:Any] => [
      def cur (state.objects get req.key)
      if (cur eq None) [ -1 ] [ size cur ]
    ]) store
    store
  ]]
  def risky fn [[b:Any store:Any] [Any] [
    def ok (do b error [ drop false ])
    if ok [ 1 ] [ 0 ]
  ]]
  def srv fn [[opts:Map] [Any] [
    def store (mk-store)
    risky [ call {op:"append" key: "k" chunk: "c"} store drop true ] store
    risky [ call {op:"append" key: "k" chunk: "c"} store drop true ] store
    call {op:"size" key: "k"} store
  ]]
  export "DrfT" { srv: srv/v }
] import
`, `DrfT.srv {}`)
	// Two risky calls (1 each: handled-ok flags) then the size read: 2
	// iff BOTH appends accumulated through the handler's computed `old`
	// binding (the dyn-bound def the drift declined).
	if len(out) != 3 {
		t.Fatalf("srv result stack = %v, want three values", out)
	}
	for i, want := range []int64{1, 1, 2} {
		n, err := asIntegerVal(out[i])
		if err != nil || n != want {
			t.Fatalf("srv result %d = %v (%v), want %d", i, out[i], err, want)
		}
	}
}
