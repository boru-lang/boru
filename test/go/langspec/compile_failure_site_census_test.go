// The compile failure-site census — how much compile failure machinery is left.
//
// design/FULL-COMPILATION.0.md section 9 asks for a census of the compile failure
// surface, ratcheting down to an end state where compile failure is not reachable
// at all. This counts the RECORDER layer: every MarkUncompilable call site
// in production code. That layer is the one Stage 9 names explicitly, and
// it is the one a source scan can count exactly — a call site is a
// syntactic fact, where a compile failure REASON often is not (a majority of the
// reason strings are built at run time from a word name, so the distinct
// reason count is a property of execution, not of the source).
//
// The other two layers are measured elsewhere and deliberately not
// duplicated here: the lowerer/Finalize declines and the CompileCheck
// latches surface as compile failure REASONS in the corpus census
// (compiled_census_test.go's failureBuckets, over rows actually declined)
// and as pinned rows in the frontier ledger. This census is the static
// half — it counts machinery that exists, not machinery that fired, so it
// keeps falling even while the corpus census sits at zero.
package langspec

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// compileFailureSiteCeiling is the number of MarkUncompilable call sites in
// production Go. Monotone DOWN only; 0 at Stage 9, when the recorder's
// terminal arm emits a generic lowering instead of latching a compile failure.
// Never raise it: a new compile failure site is new debt, and the design's whole
// claim is that the count only falls.
const compileFailureSiteCeiling = 91 // 92 -> 91 on 2026-09-26 (NUR231's type half): core/go/depscalar.go's DeclineUnknownRefinement retired — a named type over a computed bound installs at RUN time from the body the run computed (OpBindTypeRun; the check pass's node forwards to the run's, so every compiled reference — a type operand, a signature slot, a typed bind — is the run's), a typed def over one records the run's own membership check (OpBindTyped, TypedBindRunMembership), and an overload set over one re-matches at run time; a signature the run builds (an inline parameter or return type over such a bound) and a fn body's per-call type def decline through the existing compile-time-word site. Before: 91 -> 92 the same day (NUR231): core/go/depscalar.go's DeclineUnknownRefinement — a TYPE over a refinement whose bound only the run knows (a named install, an inline typed def or signature type over `Integer gt (size s)`) compiled SILENTLY WRONG, the check pass's carrier baked for the bound (`def v:T 3` bound unchecked, a parameter admitted everything); it declines now. Debt written down where a wrong answer was, with its plan (the disposition census: generic, stage 6 — the run-time type install and a compiled run-time membership check); the value half of the same record compiles (the constructor records as a run-time call). Before: 92 -> 91 on 2026-09-24 (the selective freshen): compiler/go/emit.go's embed decline retired — a fn body literal embedding an enclosing binding's container compiles, its fresh push keeping the member (Program.ConstKeep). Before: 96 (2026-08-25, Stage-1 baseline) -> 93 (2026-09-04, Stage 4b: the residual-order hazard shares the residual-provenance arm at its four sites) -> 92 (2026-09-10, NUR067: await's winner-takes-all residual is RECORDED as a runtime-variadic region instead of declining the program) -> 93 the SAME DAY, and the round trip is the honest number: recording the region exposed that a region cannot carry a CALLABLE (only the interpreter re-steps one), so the wholesale compile failure came back as a narrow one at the same arity. A representation that removes a compile failure and then owes a smaller one nets zero here, and the census is right to say so -> 92 (2026-09-14, the disposition census: the live count had been 92 since the NUR067 round trip's narrow compile failure landed on an existing line, and a ceiling one above the live value lets a site-and-row pair land unnoticed; compile_failure_disposition_census_test.go pins the same 92 in both directions) -> 0 (Stage 9)

// compileFailureSites counts MarkUncompilable call sites per module, skipping test
// files (their sites are fixtures and helpers, not compiler machinery) and
// the two declarations of the method itself.
func compileFailureSites(t *testing.T) (map[string]int, int) {
	t.Helper()
	root := filepath.Join("..", "..", "..")
	byModule := map[string]int{}
	total := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			// `.claude` holds agent scratch space, and in particular the git
			// WORKTREES an isolated agent runs in — each a full copy of this
			// repo. Walking one counts every MarkUncompilable site again, per
			// worktree: measured, three of them turned a true census of 93
			// into 372 and failed this gate on work that added no compile failure at
			// all. The directory is gitignored; the walk is over the
			// filesystem, so it has to skip it explicitly.
			case ".git", ".claude", "node_modules", "vendor", "bin":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		module := moduleOf(root, path)
		for _, line := range strings.Split(string(src), "\n") {
			if !strings.Contains(line, "MarkUncompilable(") {
				continue
			}
			// The method's own declarations are not call sites.
			if strings.Contains(line, "func ") {
				continue
			}
			byModule[module]++
			total++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return byModule, total
}

// moduleOf renders the first two path segments below root ("compiler/go"),
// or the first where there is only one.
func moduleOf(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "?"
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	return parts[0]
}

func TestCompileFailureSiteCensus(t *testing.T) {
	t.Parallel()
	byModule, total := compileFailureSites(t)

	mods := make([]string, 0, len(byModule))
	for m := range byModule {
		mods = append(mods, m)
	}
	sort.Slice(mods, func(i, j int) bool {
		if byModule[mods[i]] != byModule[mods[j]] {
			return byModule[mods[i]] > byModule[mods[j]]
		}
		return mods[i] < mods[j]
	})
	parts := make([]string, len(mods))
	for i, m := range mods {
		parts[i] = m + "×" + itoa(byModule[m])
	}
	t.Logf("compile failure-site census: %d MarkUncompilable sites (%s)", total, strings.Join(parts, ", "))

	if total > compileFailureSiteCeiling {
		t.Errorf("compile failure-site census %d exceeds ceiling %d — a new compile failure was added; the count only falls: %s",
			total, compileFailureSiteCeiling, strings.Join(parts, ", "))
	}
}
