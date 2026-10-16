package native

// Equivalence harness for the function-model consolidation refactor.
//
// This test pins TWO things across every stage of the Sigs/Signatures
// merge so each stage can prove it is behaviour-preserving:
//
//  1. The SIGNATURE TABLE of every registered word — a canonical dump
//     of each word's compiled Signatures in dispatch (sorted) order:
//     arity, per-arg type path, BarrierPos, NoEval/Quote/Type arg sets,
//     FullStack/Fallback/RunInCheckMode flags, and declared Returns.
//  2. The BEHAVIOUR of a broad corpus of programs — named fns, afn /
//     => lambdas, closures, module wrappers, higher-order words, and
//     the control words whose code-body (NoEval) semantics are the
//     primary correctness risk.
//
// Both are rendered to a single deterministic string and compared
// against a golden file (testdata/fnmodel_equivalence.golden). Run with
// -update to regenerate the golden after an INTENTIONAL change:
//
//	go test ./native -run TestFnModelEquivalence -update
//
// Any unintended drift fails the test with a diff-friendly message.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	parser "github.com/boru-lang/boru/parser/go"
)

var updateGolden = flag.Bool("update", false, "update the equivalence golden file")

// dumpSignatureTable renders every registered word's compiled signatures
// in a stable, canonical form. The output is the source of truth for the
// "signature table is preserved" half of the equivalence proof.
func dumpSignatureTable(r *Registry) string {
	names := r.Defs.Names()
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		fn := r.Lookup(name)
		if fn == nil {
			continue
		}
		fmt.Fprintf(&b, "WORD %s\n", name)
		for i := range fn.Signatures {
			b.WriteString("  ")
			b.WriteString(renderSig(&fn.Signatures[i]))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// renderSig renders one compiled Signature canonically. Handler identity
// is deliberately NOT rendered (it's an opaque func pointer); we render
// only the dispatch-visible shape, which is what must stay invariant.
func renderSig(s *Signature) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, t := range s.Args {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(typePath(t))
	}
	b.WriteByte(']')
	fmt.Fprintf(&b, " barrier=%d", s.BarrierPos)
	if g, ok := s.Impl.(*GoImpl); ok {
		if g.FullStack {
			b.WriteString(" fullstack")
		}
		if g.RunInCheckMode {
			b.WriteString(" runcheck")
		}
	}
	if s.Fallback {
		b.WriteString(" fallback")
	}
	if m := renderIntSet("noeval", s.NoEvalArgs); m != "" {
		b.WriteString(" " + m)
	}
	if m := renderIntSet("noevalmap", s.NoEvalMapArgs); m != "" {
		b.WriteString(" " + m)
	}
	if m := renderIntSet("quote", s.QuoteArgs); m != "" {
		b.WriteString(" " + m)
	}
	if m := renderIntSet("typearg", s.TypeArgs); m != "" {
		b.WriteString(" " + m)
	}
	if len(s.Returns) > 0 {
		b.WriteString(" returns=[")
		for i, t := range s.Returns {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(typePath(t))
		}
		b.WriteByte(']')
	}
	if s.ReturnsFn != nil {
		b.WriteString(" +returnsfn")
	}
	return b.String()
}

func typePath(t *Type) string {
	if t == nil {
		return "<nil>"
	}
	return t.Path()
}

func renderIntSet(label string, m map[int]bool) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]int, 0, len(m))
	for k, v := range m {
		if v {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Ints(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%d", k)
	}
	return label + "={" + strings.Join(parts, ",") + "}"
}

// behaviorCorpus is the set of programs whose results must stay
// identical across the refactor. Each entry is run twice — once in
// normal mode (result stack), once it is just recorded. The cases
// deliberately span every dispatch path the consolidation touches.
var behaviorCorpus = []struct {
	name string
	src  string
}{
	// --- native words, forward + stack + infix forms ---
	{"add-forward", `add 2 3`},
	{"sub-swap", `10 sub 3`},
	{"sub-forward", `sub 10 3`},
	{"mul-stack", `4 5 mul`},
	{"nested-arith", `add (mul 2 3) (sub 10 4)`},

	// --- if: the code-body (NoEval) correctness core ---
	{"if-then", `if true 99 88 end`},
	{"if-else", `if false 99 88 end`},
	{"if-listcond-then", `if [1 gt 0] 99 88 end`},
	{"if-listcond-else", `if [1 lt 0] 99 88 end`},
	{"if-no-else-true", `if true 99 end`},
	{"if-no-else-false", `if false 99 end`},
	{"if-clauselist", `if [false "a" true "b" "c"]`},
	{"if-bodyeval", `if true [1 add 2] [10 add 20] end`},

	// --- named boru fns (compile-to-handler path already) ---
	{"named-fn-simple", `def dbl fn [[n:Integer] [Integer] [n mul 2]]  dbl 21`},
	{"named-fn-forward", `def f fn [[a:Integer b:Integer] [Integer] [a sub b]]  f 10 3`},
	{"named-fn-recursive", `def fact fn [[n:Integer] [Integer] [if (n lte 1) [1] [n mul (fact (n sub 1))]]]  fact 5`},
	{"named-fn-multi-overload", `def g fn [[a:Integer] [Integer] [a add 1] [a:String] [String] [a]]  g 41`},

	// --- afn / => lambdas and closures ---
	{"afn-direct", `([n:Integer] => [n add 1]) 41`},
	{"closure-adder", `def make-adder ([x:Integer] => [([y:Integer] => [x add y])])  def add5 (make-adder 5)  add5 3`},
	{"closure-reuse", `def make-adder ([x:Integer] => [([y:Integer] => [x add y])])  def add5 (make-adder 5)  add5 3  add5 10`},

	// --- higher-order words ---
	{"each-double", `[1 2 3] each [dup add]`},
	{"fold-sum", `fold [add] [1 2 3 4]`},
	{"fold-init", `10 fold [add] [1 2 3 4]`},

	// Module-wrapper dispatch (the capturedReg / CallBoru branch of
	// execFnDefSig) is exercised separately in the modules package —
	// see modules/fnmodel_wrapper_equivalence_test.go — because the
	// native-module resolver is wired there, not in DefaultRegistry.

	// --- stack words (FullStack path) ---
	{"dup", `5 dup add`},
	{"swap", `1 2 swap sub`},
	{"drop", `1 2 3 drop`},

	// --- usurp-style synthesized reversed wrapper would go here once built ---
}

func runCorpusEntry(t *testing.T, src string) string {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		return "REGERR: " + err.Error()
	}
	registerIOWords(r)
	toks, perr := parser.Parse(src)
	if perr != nil {
		return "PARSEERR: " + perr.Error()
	}
	out, runErr := NewTop(r).Run(toks)
	if runErr != nil {
		return "RUNERR: " + runErr.Error()
	}
	parts := make([]string, len(out))
	for i, v := range out {
		parts[i] = v.String()
	}
	return strings.Join(parts, " ")
}

func dumpBehavior(t *testing.T) string {
	var b strings.Builder
	for _, c := range behaviorCorpus {
		fmt.Fprintf(&b, "CASE %s\n  %s\n  => %q\n", c.name, c.src, runCorpusEntry(t, c.src))
	}
	return b.String()
}

// checkCorpus exercises CHECK-MODE return-type inference for fn / afn /
// closure dispatch — the path that the function-model consolidation puts
// most at risk (anonymous fns currently infer returns via
// spliceAnonCheckResult -> AnalyseFnBody; a handler-attached path would
// instead route through execMatch's carrier intercept). Rendering the
// resulting carrier stack's type paths pins that inference byte-for-byte.
var checkCorpus = []struct {
	name string
	src  string
}{
	{"check-named-fn-int", `def dbl fn [[n:Integer] [Integer] [n mul 2]]  dbl 21`},
	{"check-named-fn-untyped", `def idf fn [[n:Integer] [Integer] [n]]  idf 7`},
	{"check-afn-direct", `([n:Integer] => [n add 1]) 41`},
	{"check-closure", `def mk ([x:Integer] => [([y:Integer] => [x add y])])  def a5 (mk 5)  a5 3`},
	{"check-if-branches", `if (1 gt 0) [10] [20] end`},
	{"check-recursive", `def fact fn [[n:Integer] [Integer] [if (n lte 1) [1] [n mul (fact (n sub 1))]]]  fact 4`},
}

// runCheckEntry runs src in CHECK MODE and renders the carrier stack as
// type paths (plus any diagnostics), so the golden captures static
// return-type inference rather than runtime values.
func runCheckEntry(src string) string {
	r, err := DefaultRegistry()
	if err != nil {
		return "REGERR: " + err.Error()
	}
	registerIOWords(r)
	r.Check.Mode = true
	toks, perr := parser.Parse(src)
	if perr != nil {
		return "PARSEERR: " + perr.Error()
	}
	out, runErr := NewTop(r).Run(toks)
	if runErr != nil {
		return "RUNERR: " + strings.SplitN(runErr.Error(), "\n", 2)[0]
	}
	parts := make([]string, len(out))
	for i, v := range out {
		// In check mode the stack holds carriers; render the lattice
		// type path (the inferred type), which is the invariant we pin.
		if ParentOf(&v) != nil {
			parts[i] = ParentOf(&v).Path()
		} else {
			parts[i] = "<nil>"
		}
	}
	diags := ""
	if n := len(r.Check.Diagnostics); n > 0 {
		codes := make([]string, n)
		for i, d := range r.Check.Diagnostics {
			codes[i] = d.Code
		}
		sort.Strings(codes)
		diags = " diagnostics=[" + strings.Join(codes, ",") + "]"
	}
	return strings.Join(parts, " ") + diags
}

func dumpCheck() string {
	var b strings.Builder
	for _, c := range checkCorpus {
		fmt.Fprintf(&b, "CHECK %s\n  %s\n  => %q\n", c.name, c.src, runCheckEntry(c.src))
	}
	return b.String()
}

func TestFnModelEquivalence(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	registerIOWords(r)

	var got strings.Builder
	got.WriteString("=== SIGNATURE TABLE ===\n")
	got.WriteString(dumpSignatureTable(r))
	got.WriteString("\n=== BEHAVIOR ===\n")
	got.WriteString(dumpBehavior(t))
	got.WriteString("\n=== CHECK MODE ===\n")
	got.WriteString(dumpCheck())

	goldenPath := filepath.Join("testdata", "fnmodel_equivalence.golden")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got.String()), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated golden: %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if got.String() != string(want) {
		t.Errorf("equivalence drift vs golden.\n%s", firstDiff(string(want), got.String()))
	}
}

// firstDiff returns a compact line-level diff context around the first
// differing line, so a drift failure points straight at what changed.
func firstDiff(want, got string) string {
	wl := strings.Split(want, "\n")
	gl := strings.Split(got, "\n")
	n := len(wl)
	if len(gl) < n {
		n = len(gl)
	}
	for i := 0; i < n; i++ {
		if wl[i] != gl[i] {
			lo := i - 3
			if lo < 0 {
				lo = 0
			}
			var b strings.Builder
			fmt.Fprintf(&b, "first diff at line %d:\n", i+1)
			for j := lo; j <= i; j++ {
				fmt.Fprintf(&b, "  want[%d]: %s\n", j+1, wl[j])
				fmt.Fprintf(&b, "  got [%d]: %s\n", j+1, gl[j])
			}
			return b.String()
		}
	}
	if len(wl) != len(gl) {
		return fmt.Sprintf("line count differs: want %d, got %d", len(wl), len(gl))
	}
	return "(no line-level diff found; trailing whitespace?)"
}
