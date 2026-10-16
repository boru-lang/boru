package lang

import (
	"strconv"
	"strings"
	"testing"
)

// Language-level performance baseline (perf-baseline suite). Complements
// BenchmarkInterpBaseline (dispatch shapes) with the remaining pipeline
// stages and word families: static check, collection words, string words,
// and sorting. Run with:
//
//	go test -bench 'BenchmarkPerf' -benchmem -run '^$' .
//
// or `make bench` from the repo root. Together the suites are the
// performance baseline: run them before and after an engine change and
// compare with benchstat.

func benchStrList(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "'s" + strconv.Itoa(i%97) + "'"
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// reversed int list — worst-case-ish input for sort.
func benchRevIntList(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = strconv.Itoa(n - i)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

var perfBaselineBenches = []baselineBench{
	// Collection words over a 500-element list.
	{"sort_int500", `def xs ` + benchRevIntList(500), `sort xs`},
	{"sort_str500", `def ss ` + benchStrList(500), `sort ss`},
	{"filter_int500", `def xs ` + benchRevIntList(500), `filter [gt 250] xs`},
	{"each_int500", `def xs ` + benchRevIntList(500), `each [add 1] xs`},
	{"fold_int500", `def xs ` + benchRevIntList(500), `fold [add] xs 0`},

	// Map construction + key access.
	{"map_build_get", "", `def m {a:1 b:2 c:3 d:4 e:5} (m.a add m.e)`},

	// String words. split lives in boru:string-util; join is core.
	{"string_split_join", `import "boru:string-util" def s 'a,b,c,d,e,f,g,h,i,j'`,
		`join '-' (StringUtil.split ',' s)`},
	{"string_interp", `def nm 'world'`, "`hello ${nm} ${1 add 2}`"},
}

func BenchmarkPerfWords(b *testing.B) {
	for _, bb := range perfBaselineBenches {
		bb := bb
		b.Run(bb.name, func(b *testing.B) { benchRun(b, bb.setup, bb.src) })
	}
}

// BenchmarkPerfCheck pins the static-analysis (check mode) cost on the
// baseline dispatch shapes.
func BenchmarkPerfCheck(b *testing.B) {
	for _, bb := range baselineBenches {
		bb := bb
		b.Run(bb.name, func(b *testing.B) {
			a, err := New()
			if err != nil {
				b.Fatal(err)
			}
			if bb.setup != "" {
				if _, err := a.RunInterp(bb.setup); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := a.Check(bb.src); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
