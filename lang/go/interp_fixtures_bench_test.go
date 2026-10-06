package lang

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// BenchmarkInterpFixtures times the INTERPRETER on the three bench/interp
// fixtures (fib, loopsum, nestloop) with the parse amortised out and no
// check pass, on a fresh instance per iteration (the fixtures bind names at
// the top level, so a reused registry would run a different program the
// second time). The CLI has had no interpreter-only lane since 2026-09-19
// (CLI.md: BORU_NO_COMPILE is gone), so bench/interp/run.sh's boru-interp
// column now times the compiled lane; this is the interpreter's own number,
// and the one an interpreter change is measured against
// (design/IN-PLACE-COMPILATION.0.md §7).
func BenchmarkInterpFixtures(b *testing.B) {
	for _, name := range []string{"fib", "loopsum", "nestloop"} {
		name := name
		b.Run(name, func(b *testing.B) {
			src, err := os.ReadFile(filepath.Join("..", "..", "bench", "interp", "fixtures", name+".boru"))
			if err != nil {
				b.Fatal(err)
			}
			vals, err := parser.Parse(string(src))
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				a, err := New()
				if err != nil {
					b.Fatal(err)
				}
				var out bytes.Buffer
				a.SetOutput(&out)
				b.StartTimer()
				if _, err := native.NewTop(a.registry).Run(vals); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
