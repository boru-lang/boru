package langspec

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// The gate that was missing.
//
// Every other compile gate in this package measures the SPEC CORPUS — 7,798
// rows whose median length is 40 characters. The compiler scores 95.9% there.
// On real programs it scores 56%. The gap is structural, not incidental: a
// compile failure is WHOLE-PROGRAM and latches on the FIRST construct it cannot lower
// (design/COMPILABLE-SUBSET.md §1), so a 10,000-character file compiles only
// if every construct in it does. A corpus of one-liners cannot see that, and
// until this gate existed nothing in the tree measured it — the 56% was
// discovered by hand, not by a test.
//
// So this walks the repo's OWN programs — the utils suite, the knowledge-graph
// pipeline, the example apps, the module sources — and pins which of them the
// emitter declines. It is COMPILE-ONLY: CompileCheck records through the checker
// and never executes, so the gate cannot fire a program's side effects.
//
// realProgramLedger is the inventory of programs that do NOT compile today.
// Every entry is an OPEN DEFECT against "all valid code compiles, no
// exceptions" — not a sanctioned outcome, and not an allowlist in the sense of
// permitting anything. It ratchets DOWN: the test fails when a ledgered
// program starts compiling (drop the entry) just as loudly as when an
// unledgered one starts declining.
var realProgramLedger = map[string]string{
	// RE-MEASURED 2026-09-26 with each program's imports resolved from the
	// directory it is written to run from (importBaseDir): 27 -> 4. Twenty-
	// two of the old entries were this gate's own artifact — the suites'
	// `import "./cat.boru"` never resolved from utils/tests, so the imported
	// words were undefined and the describe bodies naming them declined —
	// and one (kg/tests/resolution_test.boru) had a different blocker under
	// the check-diagnostics label. The same day: Test.cover declares
	// CompileRunsBodyOnRegistry (cli_test, sift_test compile), an event-
	// sourced loop range start promotes to a frame local (cut_test compiles).

	// THE LAST FOUR GRADUATED 2026-09-26 (62 of 62):
	//   - design/examples/apps/mini-redis.boru and bench/networking/apps/
	//     echo_redis.boru ("check diagnostics"): the HDEL handler's false
	//     `undefined word: h2` — `(cur get f)` narrowed a def-bound dynamic
	//     DISJUNCT to get's first overload's slot (Module), because
	//     slotIsPolymorphic measured the siblings against the carrier's
	//     Parent, the bare Disjunct node, rather than its bound; the later
	//     `cur set (f) None` then declined and took `def h2` with it.
	//   - kg/tests/resolution_test.boru ("fn call operand of unknown
	//     provenance"): `each […] xs` over a dynamic xs committed to each's
	//     Map form and applyGradualContagion kept that return STRICT (a
	//     ReturnsFn sibling had made dynamicReachableReturns abandon the
	//     union), so ingest-entity's `KgEnt.distinct-sorted (each …)` stayed
	//     uncalled beside a Map and the `aliases:` member had no home. The
	//     committed return widens to dynamic(Any) now.
	//   - bench/networking/apps/echo_s3.boru ("for: body nets multiple values
	//     per iteration"): mini-s3-client's `Net.send-bytes (slice i hi body)
	//     sock` over an `Any` param parked the fn value as data, netting
	//     [fn bytes sock] per iteration; a module native's fn value over an
	//     operand of unknown type now takes the bare word's no-signature
	//     recovery (a runtime re-match over the module's registry, raising the
	//     value's own uncalled_function on a runtime no-match).
	// Pins: lang/go/real_programs_s2b_test.go.

	// `kg/main.boru` GRADUATED 2026-09-19 (S1a of
	// design/FULL-COMPILATION-REPLAN.0.md): the knowledge-graph pipeline's
	// own entry point declined at `each` over a code body whose collection
	// the pass could not type; each now declares CompileDynBody, so the
	// dispatch lowers to a poly re-match over its own overloads instead of
	// declining at the ambiguous-overload gate. It declined again for one
	// merge on 2026-09-24 (NUR143's close): once a fn unit's
	// enclosing-binding snapshot read the unit's OWN registry, gomod.boru's
	// candidate-bundle body literal `[[repo-entity] …]` — the interpreter's
	// per-call outer list over the SHARED module-level map — was seen for
	// what it is, which the fn-unit const rule could neither deep-freshen
	// nor share (the wrong-registry snapshot had let it be deep-cloned per
	// call, an unsound compile). GRADUATED AGAIN the same day by the
	// selective (spine-only) freshen: the fresh push clones the literal's
	// spine and keeps the embedded members (Program.ConstKeep,
	// core.CloneValueKeeping).
}

// realProgramRoots are the directories holding programs a developer actually
// wrote and runs, as opposed to spec fixtures. Module FRAGMENTS (files that
// reference their importer's words and fail identically on both lanes) are
// excluded by the check-error arm below rather than by listing them.
var realProgramRoots = []string{
	"utils",
	"kg",
	"design/examples/apps",
	"lang/go/modules",
	"calc",
	"bench/networking/apps",
}

func TestRealProgramsCompile(t *testing.T) {
	t.Parallel()
	repo := filepath.Join("..", "..", "..")

	type result struct{ path, reason string }
	var declined, compiled []result

	for _, root := range realProgramRoots {
		walkRoot := filepath.Join(repo, root)
		if _, err := os.Stat(walkRoot); err != nil {
			continue
		}
		err := filepath.Walk(walkRoot, func(p string, fi os.FileInfo, e error) error {
			if e != nil || fi.IsDir() || !strings.HasSuffix(p, ".boru") {
				return nil
			}
			rel, rerr := filepath.Rel(repo, p)
			if rerr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)

			src, rerr := os.ReadFile(p)
			if rerr != nil {
				return nil
			}
			a, nerr := lang.New(lang.Options{})
			if nerr != nil {
				t.Fatalf("lang.New: %v", nerr)
			}
			a.NativeRegistry().BaseDir = importBaseDir(repo, p, string(src))

			prog, reason, _, cerr := a.CompileCheck(string(src))
			switch {
			case cerr != nil:
				// A real program either compiles or declines; an ERROR from
				// CompileCheck (a parse failure, an analysis error) is a
				// regression, never a third bucket that silently drops the file
				// from both counts (a Codex review of #471). No discovered file
				// errors today; a module fragment that resolves its importer's
				// words would need an explicit, named exclusion here, not a
				// blanket one.
				t.Errorf("%s: CompileCheck error — a real program must compile or report a clean compile failure, never error: %v", rel, cerr)
				return nil
			case prog != nil:
				compiled = append(compiled, result{rel, ""})
			default:
				declined = append(declined, result{rel, reason})
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}

	sort.Slice(declined, func(i, j int) bool { return declined[i].path < declined[j].path })

	total := len(compiled) + len(declined)
	if total == 0 {
		t.Fatal("no real programs discovered — the roots are wrong, and a gate that measures nothing passes vacuously")
	}
	t.Logf("real programs: %d compiled, %d FAILED to compile (%d total, %.1f%% compiled)",
		len(compiled), len(declined), total, 100*float64(len(compiled))/float64(total))

	byReason := map[string]int{}
	for _, r := range declined {
		byReason[r.reason]++
	}
	reasons := make([]string, 0, len(byReason))
	for r := range byReason {
		reasons = append(reasons, r)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if byReason[reasons[i]] != byReason[reasons[j]] {
			return byReason[reasons[i]] > byReason[reasons[j]]
		}
		return reasons[i] < reasons[j]
	})
	for _, r := range reasons {
		t.Logf("  failure cause x%-3d %s", byReason[r], r)
	}

	// A program that declines and is NOT ledgered is a regression.
	seen := map[string]bool{}
	for _, r := range declined {
		seen[r.path] = true
		want, ledgered := realProgramLedger[r.path]
		if !ledgered {
			t.Errorf("%s no longer compiles: %q\n"+
				"    A real program stopped compiling. Fix the construct, or — if this is a\n"+
				"    deliberate, argued step backwards — add it to realProgramLedger WITH the\n"+
				"    reason and a note saying why, and understand you are recording a defect.",
				r.path, r.reason)
			continue
		}
		if want != r.reason {
			t.Errorf("%s fails to compile for a DIFFERENT reason than ledgered:\n  ledgered: %s\n  actual:   %s\n"+
				"    The old blocker moved or a new one surfaced first. Update the entry.",
				r.path, want, r.reason)
		}
	}

	// A ledgered program that now compiles is progress — ratchet it down.
	for path := range realProgramLedger {
		if !seen[path] {
			t.Errorf("%s is ledgered as failing to compile but now COMPILES (or is no longer discovered).\n"+
				"    This is the good direction — delete its realProgramLedger entry so the gate\n"+
				"    holds the gain. A ledger that keeps stale entries stops measuring anything.",
				path)
		}
	}
}

// relativeImportRE matches a source line importing a file by a relative path
// (`import "./cat.boru"`); a `boru:` module import is not a file.
var relativeImportRE = regexp.MustCompile(`(?m)^\s*import\s+"(\.\.?/[^"]+)"`)

// importBaseDir is the directory the program's relative imports resolve
// against: a file import is resolved against the registry's BaseDir — the
// process's working directory under `boru run` / `boru test`
// (resolveImportPath) — NOT the importing file's directory, and the repo's
// programs are written for the directory their header names (`utils/tests/
// cat_test.boru` imports "./cat.boru" and runs from `utils/`; `kg/tests/
// *_test.boru` import "./schema.boru" and run from `kg/`; the bench apps
// import "./design/examples/apps/…" and run from the repo root). Until
// 2026-09-26 this gate used the file's own directory for every program, so
// 24 of its 25 ledgered "failures" were the imports not resolving there: the
// imported words undefined, and the describe bodies that named them declined
// on the way. The base is the nearest ancestor of the file's directory (up to
// the repo root) where EVERY relative import resolves; a program with no
// relative import, or none that resolves anywhere, keeps its own directory.
func importBaseDir(repo, path, src string) string {
	dir := filepath.Dir(path)
	rels := relativeImportRE.FindAllStringSubmatch(src, -1)
	if len(rels) == 0 {
		return dir
	}
	absRepo, err := filepath.Abs(repo)
	if err != nil {
		return dir
	}
	for cand := dir; ; cand = filepath.Dir(cand) {
		all := true
		for _, m := range rels {
			if _, err := os.Stat(filepath.Join(cand, m[1])); err != nil {
				all = false
				break
			}
		}
		if all {
			return cand
		}
		absCand, err := filepath.Abs(cand)
		if err != nil || absCand == absRepo || filepath.Dir(cand) == cand {
			return dir
		}
	}
}
