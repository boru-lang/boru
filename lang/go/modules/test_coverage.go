package modules

import (
	"sort"
	"sync"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// boru:test line-coverage feature. `Test.cover [body]` runs a test body with the
// engine coverage hook (eng/go/coverage.go) armed, accumulating the source rows
// the module-under-test executes; `Test.coverage <id>` reports the percentage
// against the module's DENOMINATOR — every executable row of its registered
// source (a module loader calls RegisterCoverSource + SetCoverID; boru:sift and
// every user file-module do). The hook fires from BOTH engines — the compiled
// VM (the normal, fast mode: a module fn stamped to a bytecode unit) and the
// interpreter step loop (an unstamped/uncompilable fn) — into one collector.
//
// The two engines' covered sets are CLOSE but not identical: the compiled set
// is a SUBSET of the interpreter's. The tree-walker steps every source token,
// so its rows are maximal; the VM folds some source positions (a trailing
// bare-word return compiles into the preceding expression's result, carrying no
// distinct instruction for its own row), so compiled coverage can miss rows the
// interpreter records — never the reverse. Measure with the interpreter
// (`boru test --coverage --no-compile`, or a Test.cover run on the interpreter)
// when the goal is to drive a module to 100%.

const capTestCover = "Test.cover.active"

// testCover accumulates covered rows per cover id across a coverage run.
type testCover struct {
	mu   sync.Mutex
	rows map[string]map[int]bool
}

func (c *testCover) record(id string, row int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rows == nil {
		c.rows = map[string]map[int]bool{}
	}
	m := c.rows[id]
	if m == nil {
		m = map[int]bool{}
		c.rows[id] = m
	}
	m[row] = true
}

// coveredRows returns a COPY of the rows recorded for id (empty when none).
func (c *testCover) coveredRows(id string) map[int]bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[int]bool{}
	for row := range c.rows[id] {
		out[row] = true
	}
	return out
}

// activeCover returns the testCover on the parent registry, lazily creating it.
func activeCover(parent *native.Registry) *testCover {
	if c, ok, _ := core.Cap[*testCover](parent, capTestCover); ok && c != nil {
		return c
	}
	c := &testCover{}
	_ = parent.Capabilities.Set(capTestCover, c)
	return c
}

// ArmCoverageCollector arms reg's coverage hook to record into the SAME
// collector CoverageFor / Test.coverage report from, returning the disarm func.
// The `boru test --coverage` runner arms it BEFORE running a test file so that
// (a) reg.CoverageArmed() is true when the file imports a user module — which
// makes loadFileModule tag the module and register its source (feature C) —
// and (b) the module's executed rows accumulate for CoverageFor to report. It
// is the external twin of what Test.cover arms around its own body.
func ArmCoverageCollector(reg *native.Registry) func() {
	return reg.ArmCoverageHook(activeCover(reg).record)
}

// CoverageFor reports executable-line coverage for a registered cover source id
// against the rows recorded into reg's active collector — the Go twin of the
// Test.coverage word, for the boru test --coverage runner. It returns the SORTED
// covered and uncovered executable row numbers (so the runner can both count
// them and colour each source line in an HTML report), and ok. The covered
// count is len(covered) and the total is len(covered)+len(uncovered). ok is
// false when no source is registered for id; both slices are nil when the
// registered source parses to no executable rows.
func CoverageFor(reg *native.Registry, id string) (covered, uncovered []int, ok bool) {
	src, has := reg.CoverSource(id)
	if !has {
		return nil, nil, false
	}
	denom := coverDenominator(reg, src)
	rows := activeCover(reg).coveredRows(id)
	for row := range denom {
		if rows[row] {
			covered = append(covered, row)
		} else {
			uncovered = append(uncovered, row)
		}
	}
	sort.Ints(covered)
	sort.Ints(uncovered)
	return covered, uncovered, true
}

// coverDenominator parses src and returns the set of EXECUTABLE rows: every
// token's row, with fn param/return signature declarations excluded (they are
// never stepped, so they are not coverable). Returns nil on a parse failure.
func coverDenominator(parent *native.Registry, src string) map[int]bool {
	if parent.ParseFunc == nil {
		return nil
	}
	toks, err := parent.ParseFunc(src)
	if err != nil {
		return nil
	}
	rows := map[int]bool{}
	coverWalkSeq(toks, rows)
	return rows
}

// coverWalkSeq collects the row of every executable token in seq. A `fn`/`afn`
// word's following list is a signature: its param/return sub-lists are skipped
// and only the bodies are walked (recursively — nested fns included).
func coverWalkSeq(seq []native.Value, rows map[int]bool) {
	for i := 0; i < len(seq); i++ {
		v := seq[i]
		if native.IsWord(v) {
			if r := v.Pos().Row; r != 0 {
				rows[r] = true
			}
			w, _ := native.AsWord(v)
			if (w.Name == "fn" || w.Name == "afn") && i+1 < len(seq) {
				if lst, err := native.AsList(seq[i+1]); err == nil && !lst.IsNil() {
					coverWalkFnSig(lst.Slice(), rows)
					i++
				}
			}
			continue
		}
		coverWalkValue(v, rows)
	}
}

// coverWalkFnSig walks a fn signature [[params][returns][body] ...]: only the
// body (index 2 mod 3) is executable.
func coverWalkFnSig(sig []native.Value, rows map[int]bool) {
	for i := 0; i < len(sig); i++ {
		if i%3 != 2 {
			continue
		}
		if body, err := native.AsList(sig[i]); err == nil && !body.IsNil() {
			coverWalkSeq(body.Slice(), rows)
		}
	}
}

func coverWalkValue(v native.Value, rows map[int]bool) {
	if native.IsParenExpr(v) {
		toks, _ := native.AsParenExpr(v)
		coverWalkSeq(toks, rows)
		return
	}
	if lst, err := native.AsList(v); err == nil && !lst.IsNil() {
		coverWalkSeq(lst.Slice(), rows)
		return
	}
	if m, err := native.AsMap(v); err == nil && native.IsConcrete(v) {
		for _, k := range m.Keys() {
			val, _ := m.Get(k)
			coverWalkValue(val, rows)
		}
		return
	}
	if r := v.Pos().Row; r != 0 {
		rows[r] = true
	}
}

// coverNatives builds the Go-implemented coverage words. `parent` is the
// importing registry, whose shared hook holder every module sub-registry
// inherits — so arming it captures the module-under-test's executed rows.
func coverNatives(parent *native.Registry) []native.NativeFunc {
	return []native.NativeFunc{
		{
			// Test.cover [body] — run body with the coverage hook armed, recording
			// every source row the module-under-test executes IN WHATEVER MODE the
			// body runs. A module fn stamped to a VM unit records via the VM hook
			// (vm.go), an interpreted fn via the step-loop hook (engine.go); both
			// emit into one collector. The compiled set is a subset of the
			// interpreter's (the VM folds some source positions — see the file
			// header), so a run whose module fns tree-walk yields the most complete
			// line coverage. This is the PROGRAMMATIC / manual coverage form (its
			// body usually contains `import`, which is not closure-compilable, so
			// the body itself tree-walks — but the module fns it calls still run
			// compiled when stamping is on). The `boru test --coverage` runner arms
			// the same hook externally and runs whole test files COMPILED by
			// default, so a `Test.test` case body's module calls record on the VM
			// (add --no-compile for the interpreter's line-granular coverage).
			Name: "test-cover",
			Signatures: []native.Signature{{
				Args:       []*native.Type{native.TList},
				NoEvalArgs: map[int]bool{0: true},
				Returns:    []*native.Type{}, BarrierPos: -1,
				// CompileRunsBodyOnRegistry (S2b, 2026-09-26): the suite body is
				// tree-walked over the enclosing registry in BOTH modes (the
				// sub-engine below), so at module scope the dispatch bakes as a
				// plain CALL_NATIVE over the body list and the VM runs this same
				// handler — the `import` inside the body, which no closure
				// compiles, runs where the interpreter runs it. Inside a fn frame
				// the recorder keeps the decline (a body token naming a frame
				// local would resolve against the registry).
				CompileEffect: native.CompileRunsBodyOnRegistry,
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					body, err := native.RequireConcreteList(args[0], "Test.cover")
					if err != nil {
						return nil, err
					}
					cover := activeCover(parent)
					disarm := parent.ArmCoverageHook(cover.record)
					defer disarm()
					_, e := native.New(r).Run(body.Slice())
					return nil, e
				}),
			}},
		},
		{
			// Test.coverage <id> — report {covered,total,percent,uncovered} for the
			// registered source id against the rows a prior Test.cover recorded.
			Name: "test-coverage",
			Signatures: []native.Signature{{
				Args:    []*native.Type{native.TString},
				Returns: []*native.Type{native.TMap}, BarrierPos: -1,
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					id, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					src, ok := parent.CoverSource(id)
					if !ok {
						return nil, r.BoruError("test_cover_no_source", "no coverage source registered for id: "+id, "test-coverage")
					}
					denom := coverDenominator(parent, src)
					if denom == nil {
						return nil, r.BoruError("test_cover_bad_source", "coverage source failed to parse for id: "+id, "test-coverage")
					}
					covered := activeCover(parent).coveredRows(id)
					var uncovered []int
					hit := 0
					for row := range denom {
						if covered[row] {
							hit++
						} else {
							uncovered = append(uncovered, row)
						}
					}
					sort.Ints(uncovered)
					total := len(denom)
					pct := 100.0
					if total > 0 {
						pct = float64(hit) / float64(total) * 100.0
					}
					uncov := make([]native.Value, len(uncovered))
					for i, row := range uncovered {
						uncov[i] = native.NewInteger(int64(row))
					}
					out := native.NewOrderedMap()
					out.Set("covered", native.NewInteger(int64(hit)))
					out.Set("total", native.NewInteger(int64(total)))
					out.Set("percent", native.NewFloat(pct))
					out.Set("uncovered", native.NewList(uncov))
					return []native.Value{native.NewMap(out)}, nil
				}),
			}},
		},
	}
}
