package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Quantifies what the engine pool buys on the interpreter callback hot
// path (design/legacy/SUB-ENGINE-MAIN-TAPE-REVIEW.0.ignore §3): a fresh sub-engine
// pays the tape initial-size floor (1024 entries × 160-byte Values ≈
// 164KB, allocated and zeroed) on EVERY invocation; a pooled engine
// reloads its tape in place.

func benchProgram() []core.Value { return []core.Value{core.NewInteger(1)} }

func BenchmarkSubEngineFresh(b *testing.B) {
	r, err := core.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	toks := benchProgram()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := core.New(r).Run(toks); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSubEnginePooled(b *testing.B) {
	r, err := core.NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	toks := benchProgram()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := core.RunPooled(r, toks); err != nil {
			b.Fatal(err)
		}
	}
}
