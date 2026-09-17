package lang

import (
	"strconv"
	"strings"
	"testing"
)

// Bytecode Stage-0 baseline corpus (design/legacy/boru-bytecode-plan.0.ignore
// Stage 0; results in design/legacy/boru-bytecode-baseline.0.ignore). One
// benchmark per dispatch shape, measuring the CURRENT tape
// interpreter so the bytecode go/no-go decision is made against
// post-tape, post-TCO numbers rather than the original report's
// estimates. Uses only the public New()/Run() API, same as
// BenchmarkParens, so the file keeps working as a baseline across
// engine changes.
//
// Shapes whose source is a single expression are wrapped in `for N
// [...]` loops so per-Run parse cost amortises across iterations;
// the parser's residual share is reported separately in the baseline
// doc. Definitions (fns, lists, maps) run once in setup — re-running
// a `def` per iteration would grow the def stack and skew allocation
// numbers.

type baselineBench struct {
	name  string
	setup string
	src   string
}

// arithChain builds a straight-line chain of n alternating add/sub
// dispatches with bounded intermediate values (no loop, no parse
// amortisation — this is the one shape where parse share is part of
// the question, mirroring the report's §7.1 `1 add 2` walkthrough).
func arithChain(n int) string {
	var sb strings.Builder
	sb.WriteString("0")
	for i := 0; i < n/2; i++ {
		sb.WriteString(" add 7 sub 3")
	}
	return sb.String()
}

// intList builds a literal `[0 1 2 … n-1]`.
func intList(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = strconv.Itoa(i)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

var baselineBenches = []baselineBench{
	// Straight-line arithmetic: 64 dispatches, forward collection on
	// every one (`add 7` parks the word, collects the literal).
	{"arith_chain64", "", arithChain(64)},

	// Comparison chain inside a counted loop.
	{"compare_loop", "", `for 200 [gt i 100]`},

	// Branching: scalar (paren) condition vs list-body condition.
	{"if_scalar", "", `for 200 [if (gt i 100) [1] [0]]`},
	{"if_listcond", "", `for 200 [if [i gt 100] [1] [0]]`},

	// Counted loop with a tight arithmetic body (mark/move + two
	// dispatches per iteration).
	{"for_tight", "", `for 200 [add (mul i 3) 7]`},

	// Higher-order words over a typed list.
	{"each_list", `def xs ` + intList(100), `each [mul 2] xs`},
	{"fold_int", `def xs ` + intList(100), `fold [add] xs 0`},

	// Record/map field access through a known-key dot chain.
	{"map_get", `def m {a: {b: {c: {d: 1}}}}`, `for 100 [m.a.b.c.d]`},

	// String op: handler-dominated shape (join allocates).
	{"string_join", `def ws ['alpha' 'beta' 'gamma' 'delta']`,
		`for 50 [join '-' ws]`},

	// Recursion: non-tail (frames nest on the tape) and tail
	// (TCO frame replacement) — the shapes the gap-buffer tape and
	// TCO work already optimised.
	{"recursion_nontail",
		`def s fn [[n:Integer] [Integer] [if (n lte 0) [0] [n add (s (n sub 1))]]] end`,
		`s 200`},
	{"recursion_tail",
		`def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] end`,
		`s2 1000 0`},

	// do-heavy orchestration: dynamic body evaluation per iteration —
	// the shape that stays interpreted under the bytecode design.
	{"do_body", `def body [add 1 2]`, `for 100 [do body]`},
}

func BenchmarkBytecodeBaseline(b *testing.B) {
	for _, bb := range baselineBenches {
		bb := bb
		b.Run(bb.name, func(b *testing.B) { benchRun(b, bb.setup, bb.src) })
	}
}
