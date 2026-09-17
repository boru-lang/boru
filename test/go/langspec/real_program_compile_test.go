package langspec

import (
	"os"
	"path/filepath"
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
// refusal is WHOLE-PROGRAM and latches on the FIRST construct it cannot lower
// (design/COMPILABLE-SUBSET.md §1), so a 10,000-character file compiles only
// if every construct in it does. A corpus of one-liners cannot see that, and
// until this gate existed nothing in the tree measured it — the 56% was
// discovered by hand, not by a test.
//
// So this walks the repo's OWN programs — the utils suite, the knowledge-graph
// pipeline, the example apps, the module sources — and pins which of them the
// emitter refuses. It is COMPILE-ONLY: CompileCheck records through the checker
// and never executes, so the gate cannot fire a program's side effects.
//
// realProgramLedger is the inventory of programs that do NOT compile today.
// Every entry is an OPEN DEFECT against "all valid code compiles, no
// exceptions" — not a sanctioned outcome, and not an allowlist in the sense of
// permitting anything. It ratchets DOWN: the test fails when a ledgered
// program starts compiling (drop the entry) just as loudly as when an
// unledgered one starts refusing.
var realProgramLedger = map[string]string{
	// The check-diagnostics sentinel: the checker produced findings, so the
	// emitter refuses the whole program. This is the single largest blocker
	// for real code — 13 of the 27, more than the next three causes combined.
	"bench/networking/apps/echo_redis.boru": "check diagnostics",
	"bench/networking/apps/echo_s3.boru":    "check diagnostics",
	"bench/networking/apps/echo_todo.boru":  "check diagnostics",
	"design/examples/apps/mini-redis.boru":  "check diagnostics",
	"kg/tests/codegraph_test.boru":          "check diagnostics",
	"kg/tests/digest_test.boru":             "check diagnostics",
	"kg/tests/gomod_test.boru":              "check diagnostics",
	"kg/tests/identifiers_test.boru":        "check diagnostics",
	"kg/tests/queries_test.boru":            "check diagnostics",
	"kg/tests/resolution_test.boru":         "check diagnostics",
	"kg/tests/roundtrip_test.boru":          "check diagnostics",
	"kg/tests/schema_test.boru":             "check diagnostics",
	"kg/tests/validation_test.boru":         "check diagnostics",

	// Stage-2 code bodies. `test-describe`/`test-cover` are the boru:test
	// harness words, so EVERY suite written in boru is uncompilable until
	// they lower — one defect, twelve programs.
	"lang/go/modules/cli_test.boru":   "code-body word test-cover (Stage 2)",
	"lang/go/modules/sift_test.boru":  "code-body word test-cover (Stage 2)",
	"utils/tests/cat_test.boru":       "code-body word test-describe (Stage 2)",
	"utils/tests/cut_test.boru":       "code-body word test-describe (Stage 2)",
	"utils/tests/grep_test.boru":      "code-body word test-describe (Stage 2)",
	"utils/tests/head_test.boru":      "code-body word test-describe (Stage 2)",
	"utils/tests/printenv_test.boru":  "code-body word test-describe (Stage 2)",
	"utils/tests/seq_test.boru":       "code-body word test-describe (Stage 2)",
	"utils/tests/sort_test.boru":      "code-body word test-describe (Stage 2)",
	"utils/tests/tee_test.boru":       "code-body word test-describe (Stage 2)",
	"utils/tests/truefalse_test.boru": "code-body word test-describe (Stage 2)",
	"utils/tests/uniq_test.boru":      "code-body word test-describe (Stage 2)",

	// `each` over a code body — the language's basic iteration idiom, in the
	// shape real code reaches. This one refuses the knowledge-graph
	// pipeline's own entry point, a tool this repo runs in CI.
	"kg/main.boru": "code-body word each (Stage 2)",

	// A dynamic-scope def of a value the pass could not promote.
	"utils/cut.boru": "fn cli-usage-line: dynamic-scope def `ap2` of unpromoted computed value",
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
	repo := filepath.Join("..", "..", "..")

	type result struct{ path, reason string }
	var refused, compiled []result

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
			a.NativeRegistry().BaseDir = filepath.Dir(p)

			prog, reason, _, cerr := a.CompileCheck(string(src))
			switch {
			case cerr != nil:
				// A real program either compiles or refuses; an ERROR from
				// CompileCheck (a parse failure, an analysis error) is a
				// regression, never a third bucket that silently drops the file
				// from both counts (a Codex review of #471). No discovered file
				// errors today; a module fragment that resolves its importer's
				// words would need an explicit, named exclusion here, not a
				// blanket one.
				t.Errorf("%s: CompileCheck error — a real program must compile or refuse, never error: %v", rel, cerr)
				return nil
			case prog != nil:
				compiled = append(compiled, result{rel, ""})
			default:
				refused = append(refused, result{rel, reason})
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}

	sort.Slice(refused, func(i, j int) bool { return refused[i].path < refused[j].path })

	total := len(compiled) + len(refused)
	if total == 0 {
		t.Fatal("no real programs discovered — the roots are wrong, and a gate that measures nothing passes vacuously")
	}
	t.Logf("real programs: %d compiled, %d refused (%d total, %.1f%% compiled)",
		len(compiled), len(refused), total, 100*float64(len(compiled))/float64(total))

	byReason := map[string]int{}
	for _, r := range refused {
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
		t.Logf("  refusal cause x%-3d %s", byReason[r], r)
	}

	// A program that refuses and is NOT ledgered is a regression.
	seen := map[string]bool{}
	for _, r := range refused {
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
			t.Errorf("%s refuses for a DIFFERENT reason than ledgered:\n  ledgered: %s\n  actual:   %s\n"+
				"    The old blocker moved or a new one surfaced first. Update the entry.",
				r.path, want, r.reason)
		}
	}

	// A ledgered program that now compiles is progress — ratchet it down.
	for path := range realProgramLedger {
		if !seen[path] {
			t.Errorf("%s is ledgered as refusing but now COMPILES (or is no longer discovered).\n"+
				"    This is the good direction — delete its realProgramLedger entry so the gate\n"+
				"    holds the gain. A ledger that keeps stale entries stops measuring anything.",
				path)
		}
	}
}
