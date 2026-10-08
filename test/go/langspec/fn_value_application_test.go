package langspec

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// TestFnValueApplicationCompiles pins the fn-value application milestone
// (design/legacy/STAGE3-INLINING-DESIGN-ROUND.0.ignore Stage M2, boru-bytecode-plan.0.md
// §2.4b): the `OpCallDynamic`-family lowerings compile every fn-value
// application SHAPE the corpus exercises. Positive rows must produce a native
// Program (no interpreter island); the deliberate miscompile-E auto-dispatch
// guard must keep declining the 0-arg shaped-method reads. This is the
// regression floor for the feature — a lowering that silently reverts to
// compile failure, or a weakening of the guard, trips here.
//
// It complements the parity gates (TestSpecCompiledOrFallback,
// TestPropertyDifferential) which prove the RESULTS are byte-identical: this
// asserts the COMPILATION DECISION (native vs decline) for the milestone shapes,
// so the coverage can't erode without a conscious change.
func TestFnValueApplicationCompiles(t *testing.T) {
	t.Parallel()
	// Positive: fn-value application shapes that must compile NATIVELY.
	native := []struct {
		name string
		src  string
	}{
		// M2a — apply over a Function-typed param (`/v` ref + `apply`, and a
		// param-fn threaded through a helper). recursion.tsv:90.
		{"apply-over-function-param",
			`def g ([x:Integer] => [x add 1]) def h fn [[comp:Function v:Integer] [Integer] [v comp/v apply]] def t fn [[comp:Function] [Integer] [def a (5 comp/v h) def b (7 comp/v h) a add b]] (g/v t)`},
		// M2c — shaped instance-method dispatch (l.info) + typed list-of-record
		// element reads. module-log.tsv:53.
		{"instance-method-dispatch",
			`import "boru:log" ; def l (Log.with "http" {svc:"api"}) ; Log.add-sink memory/q ; Log.remove-sink console/q ; l.info "req" ; Log.dump 0 get "logger" get`},
		// M2d — fn-value-as-operand (two-lambda higher-order form).
		// corpus-core.tsv:134.
		{"fn-value-as-operand-walk",
			`walk {mode: "depth"} {a:{x:1}} (m:Any => [m.path print]) (m:Any => [m.path print])`},
	}
	for _, c := range native {
		t.Run(c.name, func(t *testing.T) {
			prog, reason, _, err := compileRow(t, c.src)
			if err != nil {
				t.Fatalf("check error: %v", err)
			}
			if prog == nil {
				t.Fatalf("fn-value application regressed to a compile failure (reason %q): %s", reason, c.src)
			}
			if strings.Contains(prog.Disassemble(), "FALLBACK") {
				t.Errorf("fn-value application compiled with an interpreter island; expected fully native:\n%s", prog.Disassemble())
			}
		})
	}

	// A 0-arg shaped-method read now COMPILES through a real runtime
	// auto-dispatch model (not a const-fold): NoteMethodShape annotates the
	// genuine-0-arg delegation member, the read guards skip the annotated
	// read, and the landing lowers to an explicit arity-0 OpCallDynMethod
	// whose operand is the RUNTIME member value — nothing of the check-time
	// shape state bakes (the freeze-gate holds; the recorded program carries
	// a CALL_DYN_METHOD, pinned here, and the differential pins the value).
	guarded := []struct {
		name string
		src  string
	}{
		{"rand-zero-arg-method-bool", `import "boru:rand"  def r (Rand.with-seed 7)  r.bool`},
		{"rand-zero-arg-method-float", `import "boru:rand"  def r (Rand.with-seed 1)  r.float`},
	}
	for _, c := range guarded {
		t.Run(c.name, func(t *testing.T) {
			prog, reason, _, err := compileRow(t, c.src)
			if err != nil {
				t.Fatalf("check error: %v", err)
			}
			if prog == nil {
				t.Fatalf("0-arg shaped-method landing declined (reason %q); the arity-0 OpCallDynMethod model regressed: %s", reason, c.src)
			}
			if !strings.Contains(prog.Disassemble(), "CALL_DYN_METHOD") {
				t.Errorf("compiled without the guarded CALL_DYN_METHOD landing — a const-fold would freeze shape state:\n%s", prog.Disassemble())
			}
		})
	}
}

// compileRow runs one source through CompileCheck against a fresh instance
// under the frozen spec clock (matching the census harness).
func compileRow(t testing.TB, src string) (*lang.Program, string, lang.CheckResult, error) {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	a.SetClock(specClock)
	return a.CompileCheck(src)
}
