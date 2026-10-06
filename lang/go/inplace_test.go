package lang

import (
	"fmt"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// End-to-end tests of the in-place compilation prototype
// (core/go/inplace.go, design/IN-PLACE-COMPILATION.0.md): each program runs
// interpreted with the legacy completion and with in-place compilation, the
// two must agree on everything a user can observe, and the path the row is
// there for must actually have been taken (a counter moved). The whole-corpus
// oracle is TestInPlaceDifferential; these rows pin the individual paths.
// Not parallel: the counters are process-wide.

type inPlacePath func(d inPlaceCounts) bool

var (
	viaForward     inPlacePath = func(d inPlaceCounts) bool { return d.completed > 0 }
	viaStack       inPlacePath = func(d inPlaceCounts) bool { return d.stack > 0 }
	viaCommit      inPlacePath = func(d inPlaceCounts) bool { return d.normAt[core.NormCommit] > 0 }
	viaImplicitEnd inPlacePath = func(d inPlaceCounts) bool { return d.normAt[core.NormImplicit] > 0 }
	viaGenFallback inPlacePath = func(d inPlaceCounts) bool { return d.genFallback > 0 }
	viaReplan      inPlacePath = func(d inPlaceCounts) bool { return d.replanFallback > 0 }
	viaCompaction  inPlacePath = func(d inPlaceCounts) bool { return d.compactions > 0 }
	viaEagerDrop   inPlacePath = func(d inPlaceCounts) bool { return d.nops > 0 && d.compactions == 0 }
)

func TestInPlaceAgreesWithLegacy(t *testing.T) {
	long := strings.Repeat("1 drop ", 40)
	var below strings.Builder
	for k := 1; k <= 30; k++ {
		fmt.Fprintf(&below, "%d ", k)
	}
	rows := []struct {
		name, src string
		path      inPlacePath
	}{
		// The two compilation sites and the three result placements.
		{"forward call", `add 1 2 7 8 9`, viaForward},
		{"mixed call", `10 sub 3`, viaForward},
		{"value-stack call", `1 2 add`, viaStack},
		{"nullary call", `def k fn [[] [Integer] [7]] k`, viaStack},
		{"zero results leave a nop", `1 drop 2`, viaStack},
		{"several results replace the cell", `1 2 swap`, viaStack},
		{"a user fn's frame replaces the cell", `def f fn [[n:Integer] [Integer] [n mul 2]] f 21`, viaForward},
		{"more operands than the inline storage", `def f4 fn [[a:Integer b:Integer c:Integer d:Integer] [Integer] [a add b add c add d]] f4 1 2 3 4`, viaForward},
		{"paren operand", `add 1 (mul 2 3)`, viaForward},
		{"tail recursion replaces frames", `def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] end s2 300 0`, viaForward},
		{"a loop region inside a tail call's span declines the elision", `def g fn [[a:Integer b:Integer] [Integer] [a add b]] def f fn [[] [Integer] [5 for 1 [g 1]]] f`, viaForward},
		{"a loop accumulator", `def t 0 for 50 [def t (t add i)] t`, viaForward},
		// The legacy completion takes over where the plan is not the re-plan's.
		{"the word rebound among its operands", `def f fn [[a:Integer b:Integer] [Integer] [a add b]] f 1 (do [def f fn [[a:Integer b:Integer] [Integer] [a mul b]] 3])`, viaGenFallback},
		{"a pattern the plan did not test", `def f fn [[o:{pretty:Boolean}][Boolean][o.pretty]] f {prety:true}`, viaReplan},
		{"a tnot-List input", `fn List Any [1]`, viaReplan},
		// The cold paths normalise to the legacy layout. A statement end, a
		// paren close, the end of input and a loop's move never find an
		// in-place forward pending under the strict forward barrier (the
		// plan refuses first); core/go's own tests drive those sites.
		{"a barrier commit", `if (gt 3 2) [7] def q 5 q`, viaCommit},
		{"an arrival the next slot refuses", `def sub2 fn [[a:Integer b:Integer][Integer][a sub b]]  usurp sub2/v 10 3`, viaImplicitEnd},
		// A neighbour test reads past nops: a reach-read fn with a 0-arg overload.
		{"reach-read fn with a 0-arg overload", `import module [ def w fn [[] [String] ["zero"]  [x:String] [String] [x]] export "M" {w: w/v} ] end  M.w "one"`, viaForward},
		{"an in-place forward collecting a 0-arg reach-read fn", `def g fn [[a:Any b:Any] [Any] [b]] def m {f: (fn [[] [Integer] [5]])} g 1 m.f`, viaForward},
		// A call drops its own nops when it runs, so a statement stream
		// leaves none; a loop body taking a value-stack operand from below
		// its mark claims across the mark, and those nops pile up until the
		// statement compaction drops them.
		{"a statement stream leaves no nops", long + `add 1 2`, viaEagerDrop},
		{"a loop body's operands from below its mark", below.String() + `for 30 [add 10]`, viaCompaction},
	}
	for _, r := range rows {
		before := inPlaceCounterSnapshot()
		got := inPlaceRun(r.src, InPlaceOn)
		delta := inPlaceCounterSnapshot().minus(before)
		if want := inPlaceRun(r.src, InPlaceForceOff); got != want {
			t.Errorf("%s: in-place %s, legacy %s", r.name, got, want)
		}
		if !r.path(delta) {
			t.Errorf("%s: the path under test was not taken (%s)", r.name, delta)
		}
	}
}

// A printed step trace shows the cells: the word, its marker and its first
// operand as nops, and the call cell where the last operand stood.
func TestInPlaceTraceShowsCells(t *testing.T) {
	got := inPlaceRun(`import "boru:io" IO.trace (quote [add 1 2]) end`, InPlaceOn)
	if !strings.Contains(got.output, "⟨") || !strings.Contains(got.output, "·") || got.result != "[3]" {
		t.Errorf("trace: %s", got)
	}
	legacy := inPlaceRun(`import "boru:io" IO.trace (quote [add 1 2]) end`, InPlaceForceOff)
	if strings.Contains(legacy.output, "⟨") || legacy.result != got.result {
		t.Errorf("legacy trace: %s", legacy)
	}
}

// The verify lane runs the legacy completion and counts the re-plans that
// chose the plan's signature.
func TestInPlaceVerifyLane(t *testing.T) {
	core.TakeInPlaceDisagreements()
	before := inPlaceCounterSnapshot()
	if got := inPlaceRun(`add 1 2`, InPlaceVerify); got.result != "[3]" {
		t.Fatalf("verify lane: %s", got)
	}
	if d := inPlaceCounterSnapshot().minus(before); d.verified == 0 || d.calls != 0 {
		t.Errorf("the verify lane compares and compiles nothing: %s", d)
	}
	// A commit whose re-step takes a longer overload is a disagreement the
	// legacy lane resolves (IN-PLACE-COMPILATION.0.md, the verify results).
	inPlaceRun(`5 if (1 eq 1) [99] def x 1`, InPlaceVerify)
	ds := core.TakeInPlaceDisagreements()
	if len(ds) == 0 || !ds[0].Commit || !ds[0].PlanHeld {
		t.Errorf("findings %+v", ds)
	}
}

func TestInPlaceOptions(t *testing.T) {
	for in, want := range map[string]InPlaceOption{"on": InPlaceOn, "verify": InPlaceVerify, "off": InPlaceForceOff} {
		m, err := ParseOptions("inplace:" + in)
		if err != nil {
			t.Fatal(err)
		}
		var o Options
		if err := ApplyOptions(&o, m); err != nil || o.InPlace != want {
			t.Errorf("inplace:%s gave %d (%v)", in, o.InPlace, err)
		}
	}
	m, _ := ParseOptions("inplace:sometimes")
	var o Options
	if err := ApplyOptions(&o, m); err == nil || !strings.Contains(err.Error(), "on, verify or off") {
		t.Errorf("a bad inplace value must be refused: %v", err)
	}
	for opt, want := range map[InPlaceOption]core.InPlaceMode{InPlaceOn: core.InPlaceOn, InPlaceVerify: core.InPlaceVerify, InPlaceForceOff: core.InPlaceOff} {
		a, err := New(Options{InPlace: opt})
		if err != nil {
			t.Fatal(err)
		}
		if got := a.NativeRegistry().InPlace; got != want {
			t.Errorf("Options.InPlace %d set the registry to %d", opt, got)
		}
	}
	// A module sub-registry follows its parent's switch.
	if got := inPlaceRun(`import module [ def f fn [[n:Integer] [Integer] [n add 1]] export "M" {f: f/v} ] end M.f 1`, InPlaceOn); got.result != "[2]" {
		t.Errorf("module fn: %s", got)
	}
}
