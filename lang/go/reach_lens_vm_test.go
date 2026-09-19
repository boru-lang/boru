package lang

import (
	"fmt"
	"sync"
	"testing"
)

// A Reach LENS is a callable value exactly as a fn value is, and every
// consumer — `apply`, the lens forms of each/filter/sortby, getpath/setpath —
// funnels through core.ApplyReach. That primitive used to run the lowered
// `[recv dot key …]` chain on a pooled sub-engine, once per application: an
// interpreter entry inside a native handler, which TestCompiledCoverage cannot
// see (these programs disassemble with fallbacks=0). core/go/reach_unit.go
// compiles the chain and caches the unit on the Reach payload.
//
// The census ratchet is the standing gate on that; these are the direct pins.

// The compiled run must not re-enter the interpreter for a lens application,
// and it must still answer what the interpreter answers. Both halves matter:
// the seam assertion alone would pass for a program that stopped working, and
// the parity assertion alone is what "0 islanded" already claimed while every
// one of these rows interpreted.
func TestLensAppliesOnTheVM(t *testing.T) {
	for _, src := range []string{
		`def p {name:"ada" age:36}  p $.name apply`,
		`def p {a:{b:7}}  p $.a.b apply`,
		`def people [{name:"ada" age:36} {name:"bob" age:20}]  each $.name people`,
		`def xs [{n:"a" on:true} {n:"b" on:false}]  each $.n (filter $.on xs)`,
		`import "boru:struct-util"  StructUtil.getpath $.a.b {a:{b:7}}`,
	} {
		a := mustNew(t)
		var mu sync.Mutex
		seams := map[string]int{}
		disarm := a.ArmInterpEntryHook(func(ev InterpEntry) {
			if ev.CheckMode || ev.Attribution != "" {
				return
			}
			mu.Lock()
			seams[ev.Seam]++
			mu.Unlock()
		})
		gotC, compiled, errC := a.RunCompiled(src)
		if noteCompileDefect(t, src, gotC, errC) {
			continue
		}
		disarm()
		if !compiled {
			t.Errorf("%q: expected the program to run compiled", src)
			continue
		}
		mu.Lock()
		pooled := seams["runPooledSub"]
		mu.Unlock()
		if pooled != 0 {
			t.Errorf("%q: %d interpreter sub-runs — the lens did not take its unit", src, pooled)
		}

		b := mustNew(t)
		gotI, errI := b.RunInterp(src)
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
			t.Errorf("%q: engine divergence: compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
		}
	}
}

// The cache is on the PAYLOAD, so a lens applied per element compiles once. A
// per-application stamp would be a fork and a whole compile pass each time —
// the cost that sank an earlier per-const stamping attempt outright.
func TestLensStampsOncePerLens(t *testing.T) {
	a := mustNew(t)
	disarm := a.ArmRuntimeStamping()
	defer disarm()
	if _, err := a.RunInterp(`def people [{n:1} {n:2} {n:3} {n:4} {n:5}]  each $.n people`); err != nil {
		t.Fatalf("run: %v", err)
	}
	// One stamp attempt for the one lens, whatever the element count. Other
	// stamps in the program are possible, so this asserts the SHAPE: far fewer
	// attempts than applications.
	if n := len(a.StampReport()); n > 2 {
		t.Errorf("%d stamp attempts for 5 applications of one lens — the payload cache is not holding", n)
	}
}

// A lens applied while runtime stamping is UNARMED keeps the interpreted
// chain: unarmed is the interpreter mode, and stamping there would break the
// mode contract every other stamp site holds.
func TestLensDoesNotStampUnarmed(t *testing.T) {
	a := mustNew(t)
	if _, err := a.RunInterp(`def p {a:{b:7}}  p $.a.b apply`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := a.StampReport(); len(got) != 0 {
		t.Errorf("an unarmed run must not stamp a lens, got %+v", got)
	}
}

// A stamped lens whose unit DEFERS is a bail, not an island. `5 $.name apply`
// enters its unit and CALL_NATIVE_POLY finds no `dot` for an Integer receiver,
// so the VM records the defer and hands back internal_error for the
// interpreter to raise the canonical signature_error. The chain that follows is
// that defer's replay: it must carry the same attribution RunCompiled's
// top-level runtime-bail arm uses, or the one event is counted twice — once as
// a bail and again as an unattributed interpreter entry.
func TestLensBailReplayIsAttributed(t *testing.T) {
	a := mustNew(t)
	var bails int
	disarmBail := a.ArmRuntimeBailHook(func(BailEvent) { bails++ })
	var unattributed []string
	var attributed []string
	disarmEntry := a.ArmInterpEntryHook(func(ev InterpEntry) {
		if ev.CheckMode {
			return
		}
		if ev.Attribution == "" {
			unattributed = append(unattributed, ev.Seam)
			return
		}
		attributed = append(attributed, ev.Attribution)
	})
	_, _, err := a.RunCompiled(`5 $.name apply`)
	if noteCompileDefect(t, `5 $.name apply`, nil, err) {
		return
	}
	disarmEntry()
	disarmBail()

	if err == nil {
		t.Fatal("applying a field lens to an Integer must error")
	}
	if bails == 0 {
		t.Fatal("no defer recorded — the unit did not run, so this test is measuring the wrong thing")
	}
	if len(unattributed) != 0 {
		t.Errorf("bail replay left unattributed entries %v", unattributed)
	}
	for _, att := range attributed {
		if att != "fallback:runtime-bail" {
			t.Errorf("replay attributed %q, want fallback:runtime-bail", att)
		}
	}
	if len(attributed) == 0 {
		t.Error("the replay recorded no interpreter entry at all — the lane changed shape")
	}
}
