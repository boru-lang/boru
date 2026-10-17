package lang

import (
	"fmt"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// BenchmarkInterpExec is BenchmarkInterpBaseline with the parse taken out of
// the timed loop: each program is parsed once and its tokens run b.N times,
// so the numbers are the step loop's alone. BenchmarkInterpBaseline runs
// `a.Run(src)` per iteration and spends close to half its time in the parser
// on these shapes (design/INTERPRETER-PERF-HANDOVER.0.md), which hides a
// change to dispatch inside a parse cost it never touched. The corpus is the
// baseline's plus the three bench/interp workloads, at sizes that run in
// well under a second.
//
// A run reads its program and never writes it (TestInterpExecRerunIsStable
// pins that on every row), so one parse serves every iteration exactly as a
// fresh parse would.

var execExtraBenches = []baselineBench{
	{"fib20", `def fib fn n:Integer Integer [if (n lte 1) [n] [(fib (n sub 1)) add (fib (n sub 2))]] end`, `fib 20`},
	{"loopsum20k", ``, `def total 0  for 20000 [def total (total add i)]  total`},
	{"nestloop150", ``, `def total 0  for 150 [for 150 [def total (total add 1)]]  total`},
}

func execBenches() []baselineBench {
	return append(append([]baselineBench(nil), baselineBenches...), execExtraBenches...)
}

// execPrepared is one prepared program: an instance with its setup run and
// the program's parsed tokens.
type execPrepared struct {
	a    *Boru
	src  string
	vals []native.Value
}

func prepareExec(tb testing.TB, setup, src string) execPrepared {
	tb.Helper()
	a, err := New()
	if err != nil {
		tb.Fatal(err)
	}
	if setup != "" {
		if _, err := a.Run(setup); err != nil {
			tb.Fatal(err)
		}
	}
	vals, err := parser.Parse(src)
	if err != nil {
		tb.Fatal(err)
	}
	return execPrepared{a: a, src: src, vals: vals}
}

// run is runValues after its parse: the same engine construction over the
// already-parsed tokens.
func (p execPrepared) run() ([]native.Value, error) {
	p.a.registry.Source = p.src
	eng := native.NewTop(p.a.registry)
	eng.SetSource(p.src)
	return eng.Run(p.vals)
}

func BenchmarkInterpExec(b *testing.B) {
	for _, bb := range execBenches() {
		b.Run(bb.name, func(b *testing.B) {
			p := prepareExec(b, bb.setup, bb.src)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := p.run(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestInterpExecRerunIsStable guards the hoist: a parsed program run
// repeatedly answers what a fresh parse and run answers, on every row.
func TestInterpExecRerunIsStable(t *testing.T) {
	for _, bb := range execBenches() {
		p := prepareExec(t, bb.setup, bb.src)
		fresh, err := p.a.runValues(bb.src)
		if err != nil {
			t.Fatalf("%s: fresh run: %v", bb.name, err)
		}
		for i := 0; i < 2; i++ {
			got, err := p.run()
			if err != nil {
				t.Fatalf("%s: rerun %d: %v", bb.name, i, err)
			}
			if fmt.Sprint(got) != fmt.Sprint(fresh) {
				t.Fatalf("%s: rerun %d = %v, a fresh run = %v", bb.name, i, got, fresh)
			}
		}
	}
}
