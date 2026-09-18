package lang

// MODULE-FAMILY VALUES READ LIVE (2026-09-05). An import-bound namespace
// (`IO`, `StringUtil`) or a Module descriptor (`X.$module`, `def m (module
// […])`) used as a VALUE — an eq/deq operand, a residual, a def body —
// refused "operand of unknown provenance" / "residual value not statically
// materialisable": the const gate refuses a namespace on purpose (a
// pointer-shared map of fn exports; ConstBakeable is closed to module
// instances), and a `$module` read was elided as a compile-time resolution
// whose result then had no compiled home. Now the namespace read routes to
// the live OpLookupDynScope (the check pass tags it like a module-scope
// flex), the `$module` read records its dispatch (its result takes the
// event's identity), and a root def of a compile-time module value needs no
// bind op. The identity every row pins is the boxed *ModuleDesc pointer /
// the namespace facet pointer, which the live read and the runtime `dot`
// both preserve (NUR031). The corpus rows that graduated with this live in
// lang/spec/compare-restrict.tsv and lang/spec/edge-modules-1.tsv; these
// are the shapes the corpus does not spell.

import (
	"fmt"
	"strings"
	"testing"
)

func TestModuleValueReadsCompileWithParity(t *testing.T) {
	rows := []string{
		// a namespace as a value
		`import "boru:io" IO deq IO`,
		`import "boru:io" IO`,
		`import "boru:io" 1 IO`,
		`import "boru:io" def x IO x`,
		`import "boru:io" def x IO x def x IO`,
		`import "boru:io" [IO] size`,
		// a namespace read whose binding MOVES later — refused as "residual
		// value not statically materialisable" while a module fn value was the
		// one kind with a home; now that every fn carries one, the read is
		// modelled like any other module value and the row compiles with parity
		`import "boru:io" def x IO x def x 5`,
		`import "boru:io" def x IO x undef x`,
		// the descriptor
		`import "boru:io" IO.$module`,
		`import "boru:io" def x IO.$module x`,
		`import "boru:io" def x IO.$module x def x 5`,
		`import module [export "M" {a:1}] M.$module eq M.$module`,
		`import "boru:math-util" typeof MathUtil.$module`,
		`import "boru:math-util" MathUtil.$module.name`,
		`import "boru:math-util" MathUtil.$name`,
		// inside a unit, the descriptor is the body result
		`import "boru:io" def f fn [[] [Any] [IO.$module]] (f) eq (f)`,
		`import "boru:io" def f fn [[] [Any] [IO]] f`,
		// a compile-time module value bound at the root
		`def m (module [export "X" {a: 1}]) m`,
		`def m (module [export "X" {a: 1}]) for 2 [9 m]`,
		// the export dispatch path is untouched
		`import "boru:string-util" (StringUtil.upper "x")`,
	}
	for _, src := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled {
			t.Errorf("%q: must compile natively; err=%v", src, errC)
			continue
		}
		requireParity(t, src, gotC, errC, gotI, errI)
	}
}

// The shape the live read must NOT admit: a frame-local def of a
// compile-time module value keeps its refusal (its binding is popped with
// the frame, so there is nothing for the live read to find); the interpreter
// answers. The two moved-binding rows that used to sit here (`def x IO x def
// x 5`, `… undef x`) graduated to the parity rows above once every fn value
// carried its home.
func TestModuleValueReadSoundFallbacks(t *testing.T) {
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	rows := []struct{ src, reason string }{
		{`def f fn [[] [Any] [def m (module [export "X" {a: 1}]) m]] f`, "fn f: body result of unknown provenance"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("CompileCheck(%q): %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — this shape has graduated; move it to the parity rows", c.src)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: refusal drifted: want %q in %q", c.src, c.reason, reason)
		}
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if compiled {
			t.Errorf("%q: expected the interpreter fallback", c.src)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
			t.Errorf("%q: engine divergence on the fallback: compiled=%v/%v interp=%v/%v", c.src, gotC, errC, gotI, errI)
		}
	}
}
