package lang

import (
	"testing"

	eng "github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// Stage-6 measurement harness (design/legacy/boru-bytecode-plan.0.ignore Stage 6):
// compare compiled-mode EXECUTION against the interpreter on the
// Stage-0 baseline shapes, with parse/compile cost amortised out so the
// numbers isolate the dispatch machinery the bytecode VM eliminates.
// Each shape runs in both modes on the SAME registry (setup applied
// once); /interp re-executes the pre-parsed value stream, /compiled
// re-runs the compiled Program. Shapes that do not compile are skipped
// on the /compiled side (the win there is 0 by construction — they fall
// back to the interpreter).

// benchExecInterp parses src once, then times native.NewTop(reg).Run on
// the pre-parsed values — interpreter execution without per-iter parse.
func benchExecInterp(b *testing.B, setup, src string) {
	b.Helper()
	a, err := New()
	if err != nil {
		b.Fatal(err)
	}
	if setup != "" {
		if _, err := a.RunInterp(setup); err != nil {
			b.Fatal(err)
		}
	}
	vals, err := parser.Parse(src)
	if err != nil {
		b.Fatal(err)
	}
	reg := a.registry
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := native.NewTop(reg).Run(vals); err != nil {
			b.Fatal(err)
		}
	}
}

// benchExecCompiled compiles src once, then times eng.RunProgram —
// compiled execution without per-iter compile. Skips when the program
// does not compile (it would just run the interpreter).
func benchExecCompiled(b *testing.B, setup, src string) {
	b.Helper()
	a, err := New()
	if err != nil {
		b.Fatal(err)
	}
	if setup != "" {
		if _, err := a.RunInterp(setup); err != nil {
			b.Fatal(err)
		}
	}
	prog, reason, _, err := a.CompileCheck(src)
	if err != nil {
		b.Fatalf("compile error: %v", err)
	}
	if prog == nil {
		b.Skipf("not compiled: %s", reason)
	}
	reg := a.registry
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := eng.RunProgram(prog, reg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStage6(b *testing.B) {
	for _, bb := range baselineBenches {
		bb := bb
		b.Run(bb.name+"/interp", func(b *testing.B) { benchExecInterp(b, bb.setup, bb.src) })
		b.Run(bb.name+"/compiled", func(b *testing.B) { benchExecCompiled(b, bb.setup, bb.src) })
	}
}
