// Package vary generates STRUCTURED VARIATIONS of currently-passing spec
// rows and classifies each variant through the dual interpreter/compiler
// pipeline, to find compiler gaps the curated corpus does not exercise
// (design/legacy/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.ignore, the test-first frontier
// suite's WS3).
//
// The idea: every lang/spec row is a program a human thought to write. Each
// TRANSFORM below re-embeds that program in a different compile context (a fn
// body, a do/catch, a loop body, a module body, a dirty stack…) that the
// compiler lowers through a different path. The transform does NOT need to
// preserve the seed's semantics — the interpreter re-runs every variant as
// the oracle, so an ill-formed variant is discarded (InterpReject) and a
// well-formed one gets the full native-compile + byte-parity verdict. A
// variant that DIVERGES is a miscompile (the highest-value find); a variant
// that REFUSES in a new reason bucket is a new frontier class.
//
// Consumers: the specgen `-vary` CLI mode (full-breadth triage sweeps) and
// test/go/langspec's TestVariationDifferential (the standing CI gate, modest
// deterministic breadth) share this package, so both see the same transform
// table and the same classification.
package vary

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
)

// SpecClock freezes time to the corpus instant (langspec's specClock) so
// clock-seeded behaviour is reproducible across seeds and variants.
var SpecClock = capabilities.FixedClock{T: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)}

// Test seams (design/TEST-SEAMS.10.md): swapped by unit tests to drive the
// classifier arms that a healthy build cannot reach (a genuine divergence, a
// FALLBACK island, instance-construction failure).
var (
	langNew      = func() (*lang.Boru, error) { return lang.New() }
	compileCheck = (*lang.Boru).CompileCheck
	runCompiled  = (*lang.Boru).RunCompiled
	disasm       = func(p *lang.Program) string { return p.Disassemble() }
)

// Seed is one corpus row used as variation input. Only the input matters —
// the interpreter recomputes every expectation at classification time.
type Seed struct {
	File  string
	Line  int
	Input string
}

// LoadSeeds reads the VALUE rows (expected column not ERROR:) of every
// *.tsv directly in dir — the flat glob, matching the census corpus, so
// subdirectories (lang/spec/frontier) are excluded by construction.
func LoadSeeds(dir string) ([]Seed, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var seeds []Seed
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		for i, line := range strings.Split(string(data), "\n") {
			line = strings.TrimRight(line, " \t\r")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue // not a data row (prose, section headers)
			}
			input := strings.TrimSpace(parts[0])
			expected := strings.TrimSpace(parts[1])
			if input == "" || strings.HasPrefix(expected, "ERROR:") {
				continue // error rows are never variation seeds
			}
			seeds = append(seeds, Seed{File: e.Name(), Line: i + 1, Input: input})
		}
	}
	return seeds, nil
}

// Sample returns a deterministic n-seed sample ordered by a stable hash
// priority. Samples NEST: Sample(s, n) is a prefix of Sample(s, m) for
// n < m, so cranking breadth (BORU_VARY_SEEDS) strictly ADDS variants and a
// bucket observed at the default breadth stays observed at any larger one —
// the property the CI ledger's stale arm relies on. n <= 0 or n >= len
// returns all seeds (in priority order).
func Sample(seeds []Seed, n int) []Seed {
	ordered := make([]Seed, len(seeds))
	copy(ordered, seeds)
	prio := func(s Seed) uint64 {
		h := fnv.New64a()
		h.Write([]byte(s.Input))
		return h.Sum64()
	}
	sort.Slice(ordered, func(i, j int) bool {
		pi, pj := prio(ordered[i]), prio(ordered[j])
		if pi != pj {
			return pi < pj
		}
		if ordered[i].File != ordered[j].File {
			return ordered[i].File < ordered[j].File
		}
		return ordered[i].Line < ordered[j].Line
	})
	if n <= 0 || n >= len(ordered) {
		return ordered
	}
	return ordered[:n]
}

// Transform is one structured perturbation: Wrap re-embeds a seed program in
// a different compile context. Wrappers use the zzv* name prefix to avoid
// colliding with seed bindings.
type Transform struct {
	Name string
	Wrap func(src string) string
}

// Transforms is the fixed, ordered transform table. Each entry targets a
// distinct lowering path (the plan's context-wrapping / statement-mixing /
// operand-provenance axes); growing this table is the cheap way to widen the
// sweep.
func Transforms() []Transform {
	return []Transform{
		// Paren grouping: the seed becomes one inline-evaluated group.
		{"paren-group", func(s string) string { return "(" + s + ")" }},
		// Fn-body context: the seed compiles as a user-fn UNIT (contract-free
		// returns so multi-value seeds stay valid), then the fn is called.
		{"fn-body", func(s string) string { return "def zzvfn fn [[] [] [" + s + "]] zzvfn" }},
		// Lambda construction + named dispatch.
		{"lambda-body", func(s string) string { return "def zzvlam ([] => [" + s + "]) zzvlam" }},
		// do sub-program (closure unit).
		{"do-body", func(s string) string { return "do [" + s + "]" }},
		// do under a catch: the fallible-body / variable-arity frontier.
		{"do-catch", func(s string) string { return "do [" + s + "] error [dot code]" }},
		// Branch arms: taken-then, taken-else.
		{"if-then", func(s string) string { return "if true [" + s + "] [0]" }},
		{"if-else", func(s string) string { return "if false [0] [" + s + "]" }},
		// Loop body run twice: per-iteration net accounting.
		{"for-body", func(s string) string { return "for 2 [" + s + "]" }},
		// Higher-order iteration body over a concrete list.
		{"each-body", func(s string) string { return "[10 20] each [drop " + s + "]" }},
		// Module body + export + dot-dispatch: the cross-registry path.
		{"module-body", func(s string) string {
			return `import module [ def zzvmod fn [[] [] [` + s + `]] export "ZZV" {run: zzvmod/v} ] end ZZV.run`
		}},
		// Statement mixing: an unrelated def before / after the seed.
		{"prefix-def", func(s string) string { return "def zzvpre 7 " + s }},
		{"suffix-def", func(s string) string { return s + " def zzvpost 8" }},
		// A dirty stack below the seed: the forward-drift / prefix class.
		{"prefix-stack", func(s string) string { return "7 " + s }},
		// Code splice: the seed rides a word-marker and splices at the pointer.
		{"splice", func(s string) string { return "def zzvsp word [" + s + "] zzvsp" }},
	}
}

// Outcome classifies one variant through the dual pipeline.
type Outcome int

const (
	// Pass: interpreter-clean, compiles natively (no island), runs compiled
	// with byte-identical value/error parity.
	Pass Outcome = iota
	// InterpReject: the transform produced a program the interpreter rejects
	// — not a compiler finding; discarded.
	InterpReject
	// CheckReject: the interpreter runs it but CompileCheck hard-errors (a
	// parse/check-run failure) or the harness could not be built.
	CheckReject
	// Refused: CompileCheck returned no Program (Detail = the reason), or
	// RunCompiled declined at runtime (Detail prefixed "runtime bail:").
	Refused
	// Islanded: it compiled, but the Program embeds an OpFallback span.
	Islanded
	// Diverged: it compiled and ran, and the result differs from the
	// interpreter — a MISCOMPILE.
	Diverged
	// Panicked: one of the engines PANICKED on the program — a crash in
	// the compiler or the interpreter, recovered so that one broken
	// program names itself instead of taking the whole sweep down.
	Panicked
	// Hung: no engine answered within Deadline — a program that blocks
	// (a top-level `receive` with no `after` clause waits for a message
	// that never comes). Its goroutine is abandoned so the sweep goes on.
	Hung
)

// Deadline bounds one classification. Every corpus program answers in
// milliseconds; a program that has not answered by the deadline is Hung.
// Its classification is abandoned, not stopped — the goroutine runs on
// until the engine returns, if it ever does — and inflight counts it, so a
// caller that swaps the seams (the unit tests) can wait for it to end
// before restoring them.
var Deadline = 30 * time.Second

// inflight counts classifications still running, abandoned ones included.
var inflight sync.WaitGroup

// String names the outcome for reports.
func (o Outcome) String() string {
	switch o {
	case Pass:
		return "pass"
	case InterpReject:
		return "interp-reject"
	case CheckReject:
		return "check-reject"
	case Refused:
		return "refused"
	case Islanded:
		return "islanded"
	case Diverged:
		return "DIVERGED"
	case Panicked:
		return "PANIC"
	default:
		return "HUNG"
	}
}

// Result is one variant's classification.
type Result struct {
	Outcome Outcome
	Detail  string
}

// Classify runs one program through the dual pipeline on fresh, isolated
// instances (the freshDivergence discipline — no reused-instance artifacts):
// interpreter oracle first, then CompileCheck + island scan + RunCompiled
// parity. Output is discarded so print-bearing variants stay quiet. A
// panic in either engine is recovered into Panicked, named by the phase it
// fired in, and a program that has not answered within Deadline is Hung.
func Classify(src string) Result {
	done := make(chan Result, 1)
	inflight.Add(1)
	go func() {
		defer inflight.Done()
		done <- classify(src)
	}()
	select {
	case r := <-done:
		return r
	case <-time.After(Deadline):
		return Result{Hung, fmt.Sprintf("HUNG: no answer within %s", Deadline)}
	}
}

// classify is Classify without the deadline.
func classify(src string) (res Result) {
	phase := "interp"
	defer func() {
		if r := recover(); r != nil {
			res = Result{Panicked, fmt.Sprintf("PANIC in %s: %v", phase, r)}
		}
	}()
	ai, err := langNew()
	if err != nil {
		return Result{CheckReject, "harness: " + err.Error()}
	}
	ai.SetClock(SpecClock)
	ai.SetOutput(discard{})
	gotI, errI := ai.RunInterp(src)
	if errI != nil {
		return Result{InterpReject, errI.Error()}
	}

	ac, err := langNew()
	if err != nil {
		return Result{CheckReject, "harness: " + err.Error()}
	}
	ac.SetClock(SpecClock)
	ac.SetOutput(discard{})
	phase = "compile"
	prog, reason, _, cerr := compileCheck(ac, src)
	if cerr != nil {
		return Result{CheckReject, reason + ": " + cerr.Error()}
	}
	if prog == nil {
		return Result{Refused, reason}
	}
	phase = "disassemble"
	if strings.Contains(disasm(prog), "FALLBACK") {
		return Result{Islanded, "program embeds an OpFallback island"}
	}

	ar, err := langNew()
	if err != nil {
		return Result{CheckReject, "harness: " + err.Error()}
	}
	ar.SetClock(SpecClock)
	ar.SetOutput(discard{})
	phase = "run"
	gotC, wasC, errC := runCompiled(ar, src)
	if !wasC {
		return Result{Refused, fmt.Sprintf("runtime bail: did not run compiled (err=%v)", errC)}
	}
	if fmt.Sprint(errC) != fmt.Sprint(errI) {
		return Result{Diverged, fmt.Sprintf("error divergence: compiled %v vs interp %v", errC, errI)}
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		return Result{Diverged, fmt.Sprintf("value divergence: compiled %v vs interp %v", gotC, gotI)}
	}
	return Result{Pass, ""}
}

// discard is an io.Writer sink (avoids importing io just for Discard).
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// Variant is one classified (seed × transform) program. The base seed's own
// classification appears with Transform == "seed"; transform variants are
// generated only for seeds whose base classification is Pass — variations of
// an already-refused row would only re-observe the base refusal, which the
// corpus ratchets already own.
type Variant struct {
	Seed      Seed
	Transform string
	Src       string
	Res       Result
}

// numWorkers is a test seam (drives the single-worker floor).
var numWorkers = runtime.NumCPU

// SweepSeeds classifies every seed and, for passing bases, every transform
// variant. Work fans across workers (fresh instances per Classify, so
// seed-level parallelism is artifact-free) but the returned order is
// deterministic: seeds in the given order, each base first then its variants
// in Transforms() order. report (optional) receives monotonic progress.
func SweepSeeds(seeds []Seed, report func(done, total int)) []Variant {
	transforms := Transforms()
	perSeed := make([][]Variant, len(seeds))
	workers := numWorkers()
	if workers < 1 {
		workers = 1
	}
	var next, done int64 = -1, 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(atomic.AddInt64(&next, 1))
				if i >= len(seeds) {
					return
				}
				sd := seeds[i]
				base := Classify(sd.Input)
				vs := []Variant{{Seed: sd, Transform: "seed", Src: sd.Input, Res: base}}
				if base.Outcome == Pass {
					for _, tr := range transforms {
						src := tr.Wrap(sd.Input)
						vs = append(vs, Variant{Seed: sd, Transform: tr.Name, Src: src, Res: Classify(src)})
					}
				}
				perSeed[i] = vs
				if report != nil {
					mu.Lock()
					report(int(atomic.AddInt64(&done, 1)), len(seeds))
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	var all []Variant
	for _, vs := range perSeed {
		all = append(all, vs...)
	}
	return all
}
