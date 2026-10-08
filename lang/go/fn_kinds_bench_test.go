package lang

import "testing"

// The per-KIND call cost: one shape per way a function body can be reached
// — a named fn, a def-bound lambda, a closure from a factory, a word splice,
// a `do` body, a fn value through `apply`, a module fn, a class method, and
// a code body / fn value / lambda value through `each` — each 500 calls,
// against the body written inline, on both lanes with parse and compile
// amortised out (the Stage-6 harness, bytecode_stage6_bench_test.go). The
// interpreter column is what the module-fn pooling and the leaf fast paths
// in execFnDefSig / CallBoru moved (core/go, 2026-10-08):
//
//	go test -run '^$' -bench 'BenchmarkFnKinds/.*/interp' -benchmem .
var fnKindBenches = []baselineBench{
	{"inline_body", ``, `for 500 [i add 1]`},
	{"named_fn", `def inc fn [[x:Integer][Integer][x add 1]]`, `for 500 [inc i]`},
	{"lambda", `def lam ([x:Integer] => [x add 1])`, `for 500 [lam i]`},
	{"closure", `def mk fn [[n:Integer][Function][(fn [[x:Integer][Integer][x add n]])]]  def c (mk 1)`, `for 500 [c i]`},
	{"word_splice", `def inc2 word [1 add]`, `for 500 [i inc2]`},
	{"do_body", `def body (quote [i add 1])`, `for 500 [do body]`},
	{"apply_fn_value", `def inc fn [[x:Integer][Integer][x add 1]]`, `for 500 [i inc/v apply]`},
	{"module_fn", `import module [def f fn x:Integer Integer [x add 1] export "M" {f: f/v}] end`, `for 500 [M.f i]`},
	{"method", `def C class {op:(fn [[x:Integer] [Integer] [x add 1]])} def c (make C {})`, `for 500 [c.op i]`},
	{"each_code_body", `def xs (range 0 500)`, `each [add 1] xs`},
	{"each_fn_value", `def inc fn [[x:Integer][Integer][x add 1]]  def xs (range 0 500)`, `each inc/v xs`},
	{"each_lambda_value", `def lam ([x:Integer] => [x add 1])  def xs (range 0 500)`, `each lam/v xs`},
}

func BenchmarkFnKinds(b *testing.B) {
	for _, bb := range fnKindBenches {
		bb := bb
		b.Run(bb.name+"/interp", func(b *testing.B) { benchExecInterp(b, bb.setup, bb.src) })
		b.Run(bb.name+"/compiled", func(b *testing.B) { benchExecCompiled(b, bb.setup, bb.src) })
	}
}
