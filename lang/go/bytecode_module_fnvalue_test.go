package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestModuleFnValueAsArg pins the off-corpus shape the langspec differential is
// blind to: a MODULE fn value (a real-Boru-body export, e.g. a comparator
// `M.by-num`) passed as an ARGUMENT to another fn. The value must bake as an inert
// const (isInertConst FnDefInfo case) — the sub-registry pointer it carries is the
// SAME object the compiled run shares, so check-pass and VM see one fn — and the VM
// must apply it faithfully: the island sub-engine (callDynTrailTop's
// `vc.island().Run([fn, args…])`) INTERPRETS the body in fnDef.Registry, so a
// real-body module fn applies soundly (CallBoru + module-private scope). Before the
// fn-dispatch unification + this bake relaxation, such a value declined at the call
// ("fn call operand of unknown provenance"); the comparison sorts that thread a
// `M.by-...` comparator into the sort fn all hit it.
//
// compile == interpret MUST hold; these shapes compile NATIVELY (no FALLBACK) and
// RunCompiledStrict == Run. want is the program residual (single wrapped).
func TestModuleFnValueAsArg(t *testing.T) {
	const prelude = `import module [
  def by-num fn [[b:Any a:Any] [Integer] [(a cmp b)]]
  def mapcmp fn [[comp:Function xs:List] [List] [ xs each [ var [[x] (x 5 comp) ] ] ]]
  def cmp1 fn [[comp:Function p:Integer] [Integer] [ (p 5 comp) ]]
  export "M" {by-num: by-num/v, mapcmp: mapcmp/v, cmp1: cmp1/v}
] end `
	strict := []struct{ name, src, want string }{
		// the module comparator threads into a fn that applies it under a closure
		{"module comparator into each-applying fn",
			prelude + `([3 5 9] M.mapcmp M.by-num)`, "[[-1 0 1]]"},
		// the module comparator threads into a plain (non-closure) fn body apply
		// (stack form: comp=M.by-num forward, p=9 from the stack)
		{"module comparator into direct-apply fn",
			prelude + `(9 M.cmp1 M.by-num)`, "[1]"},
		// the comparator as a bare residual still bakes (the delegation-era case)
		{"module fn value as a bare residual list member",
			prelude + `[M.by-num]`, "[[fn by-num(Any, Any)]]"},
	}
	for _, c := range strict {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog == nil {
				t.Fatalf("must compile natively, declined: %q", reason)
			}
			if strings.Contains(prog.Disassemble(), "FALLBACK") {
				t.Errorf("%s must compile native (no island in the PROGRAM — the apply uses a runtime island, not a lowered FALLBACK span)", c.name)
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict: %v", err)
			}
			b, _ := New()
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}
}
