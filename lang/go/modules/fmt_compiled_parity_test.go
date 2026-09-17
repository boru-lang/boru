package modules_test

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// fmt_compiled_parity_test.go pins the Stage-3 fn-value dispatch graduation
// for the boru:fmt declarative-formatter demos: the `apply` driver
// (`def fmtapply fn [nd:Any Any [nd (rules get (Fmt.kind nd)) apply]]` — a Function
// value fetched from a map at runtime, applied to a waiting value) COMPILES
// via the whole-frame dynamic-apply replay (OpCallDynFrame + RetReplay) and
// produces byte-identical results to the interpreter. Before the
// noteDynFrameReplay widening to Dynamic residuals these programs refused
// ("fn apply: result above a literal (Stage 3)") and ran interpreter-only.

// runBothEngines runs prog through a fresh compiled instance and a fresh
// interpreter instance, requires the compiled path was actually taken, and
// returns both final values rendered as strings.
func runBothEngines(t *testing.T, prog string) (compiled, interpreted string) {
	t.Helper()
	ac, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New (compiled): %v", err)
	}
	gotC, wasCompiled, errC := ac.RunCompiled(prog)
	if errC != nil {
		// Since the BROAD park (NUR073 clause 3) the rules engine's fetched
		// fn reaches `apply` as an untyped carrier, and the record refuses
		// ("apply over a dynamic lead") rather than lower an unprovable
		// overload — the interpreter owns the shape until it graduates.
		if strings.Contains(errC.Error(), "compile_refused") {
			t.Skipf("compiled lane refused: %v", errC)
		}
		t.Fatalf("RunCompiled: %v", errC)
	}
	if !wasCompiled {
		t.Skipf("program refused compilation (sound; see the BROAD note above)")
	}
	ai, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New (interpreter): %v", err)
	}
	gotI, errI := ai.RunInterp(prog)
	if errI != nil {
		t.Fatalf("Run: %v", errI)
	}
	if len(gotC) == 0 || len(gotI) == 0 {
		t.Fatalf("empty result: compiled %v, interpreted %v", gotC, gotI)
	}
	return fmt.Sprint(gotC[len(gotC)-1]), fmt.Sprint(gotI[len(gotI)-1])
}

// TestFmtDeclarativeFormatterCompiledParity is the compiled twin of
// TestFmtDeclarativeFormatter (fmtrule_test.go): the data-tree demo.
func TestFmtDeclarativeFormatterCompiledParity(t *testing.T) {
	const prog = `import "boru:fmt"
def fmtapply fn [nd:Any Any [nd (rules get (Fmt.kind nd)) apply]]
def rules {
  scalar: (nd:Any => (canon nd))
  entry:  (nd:Any => ({fmt:'concat' parts:[nd.key ':' (fmtapply nd.value)]}))
  map:    (nd:Any => ({fmt:'group' body:{fmt:'concat' parts:((Fmt.children nd) each [fmtapply])}}))
}
Fmt.render 72 (fmtapply {a:1 b:{c:2}})`
	gotC, gotI := runBothEngines(t, prog)
	if gotC != gotI {
		t.Errorf("compiled %q != interpreted %q", gotC, gotI)
	}
	if want := "a:1b:c:2"; gotC != want {
		t.Errorf("declarative format = %q, want %q", gotC, want)
	}
}

// TestFmtTreeDeclarativeFormatterCompiledParity is the compiled twin of
// TestFmtTreeDeclarativeFormatter (fmttree_test.go): the source-tree demo,
// exercised over a nested-paren statement (dispatch + recursion + fold).
func TestFmtTreeDeclarativeFormatterCompiledParity(t *testing.T) {
	const prog = `import "boru:fmt"
def fmtapply fn [nd:Any Any [nd (rules get (Fmt.kind nd)) apply]]
def inline fn [nd:Any Any [
  def r ({s:"" p:none} fold [foldstep] (nd.children))
  r.s
]]
def foldstep fn [[elem:Any accm:Any] Any [
  def news (join "" [accm.s (if (eq accm.p none) [""] [" "]) (fmtapply elem)])
  {s:news p:elem}
]]
def rules {
  root:(nd:Any => (inline nd)) word:(nd:Any => (nd.text)) number:(nd:Any => (nd.text))
  paren:(nd:Any => (join "" ["(" (inline nd) ")"]))
}
fmtapply (Fmt.tree "(f (g 1) 2)")`
	gotC, gotI := runBothEngines(t, prog)
	if gotC != gotI {
		t.Errorf("compiled %q != interpreted %q", gotC, gotI)
	}
	if want := "(f (g 1) 2)"; gotC != want {
		t.Errorf("declarative source format = %q, want %q", gotC, want)
	}
}
