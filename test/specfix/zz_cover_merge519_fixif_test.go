package specfix

import (
	"fmt"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"
)

// merge519FixRegistry is the standalone lanes' registry: the spec vocabulary
// plus the fixture control words.
func merge519FixRegistry(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("core.NewRegistry: %v", err)
	}
	RegisterSpecWords(r)
	RegisterControlWords(r)
	r.InitRootContext()
	return r
}

// merge519FixRecord runs src under an armed recording pass and reports the
// recorder's verdict.
func merge519FixRecord(t *testing.T, src string) (compilable bool, reason string) {
	t.Helper()
	r := merge519FixRegistry(t)
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	finish := r.Check.BeginCompilePass()
	defer finish()
	if _, err := core.NewTop(r).Run(vals); err != nil {
		t.Fatalf("record %q: %v", src, err)
	}
	es, ok := r.Check.Recorder().(*compiler.EmitState)
	if !ok {
		t.Fatalf("the recording pass must arm a compiler EmitState, got %T", r.Check.Recorder())
	}
	return es.Compilable, es.Reason
}

// TestMerge519FixIfComputedListArmDeclines pins the fixture `if`'s documented
// narrowing: a COMPUTED list arm (a fn result, not a literal body) is the
// interpreter's spliced code body, which the fixture does not model, so the
// recording pass declines the compile with a named reason instead of lowering
// it as a value. The interpreter still runs the program — the arm's body
// splices and its values land. The negative twins are a literal code-body arm
// and a computed NON-list arm, neither of which meets the decline.
func TestMerge519FixIfComputedListArmDeclines(t *testing.T) {
	const reason = "fixture if: computed list arm"
	for _, src := range []string{
		`def mk fn [[][List][[1 2]]] if true (mk) ["f"]`,
		`def c (not false) def mk fn [[][List][[1 2]]] if c (mk) ["f"]`,
	} {
		if ok, why := merge519FixRecord(t, src); ok || why != reason {
			t.Errorf("%s: want the compile declined with %q, got compilable=%v reason=%q", src, reason, ok, why)
		}
		vals, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		out, err := core.NewTop(merge519FixRegistry(t)).Run(vals)
		if err != nil || fmt.Sprint(out) != "[1 2]" {
			t.Errorf("%s: the interpreter splices the computed body: got %v / %v, want [1 2]", src, out, err)
		}
	}

	for _, src := range []string{
		`if true [1 2] ["f"]`,
		`def mk fn [[][Integer][5]] if true (mk) ["f"]`,
	} {
		if _, why := merge519FixRecord(t, src); why == reason {
			t.Errorf("%s: a literal body or a computed non-list arm must not meet the computed-list decline", src)
		}
	}
}
