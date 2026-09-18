// Spec-coverage guard for native-module exports (ADR-003).
//
// Every word exported by a native module (the ones under
// lang/go/modules/, reachable as `Namespace.word` after `import`) must be
// exercised by at least one row in the lang/spec/*.tsv suite. This test
// enumerates the live export set straight from the module registry — so a
// newly-added export cannot escape notice — and asserts each qualified
// name `Namespace.word` appears literally in some spec file's input.
//
// The check is intentionally content-based (a substring scan of the .tsv
// inputs) rather than a parse of each row: a single row whose INPUT column
// mentions the qualified name is enough to prove the word is reachable and
// tested. Only the input column of real data rows counts — comments and the
// expected/description columns are excluded, so a word cannot be "covered"
// by merely naming it in a comment. The companion TestSpecProd actually runs
// those rows; this guard only asserts they exist for every export.
package langspec

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/parser/go"
)

// hermeticExempt lists the few exports that cannot be exercised by a
// hermetic, deterministic spec row and are covered by Go tests instead.
// This is the escape hatch ADR-003 deliberately almost never allows — keep
// it TINY and justify every entry. Each key must still be a live export
// (the test fails on a stale entry) so the list can't rot.
var hermeticExempt = map[string]string{
	// IO.folder is a host-FS mkdir: the in-memory-FS toggle does not engage
	// through a spec row's context layering, and `make Path` mangles a
	// "mem://" scheme, so there is no hermetic spec surface. Covered by
	// lang/go/native/folder_test.go.
	"IO.folder": "host-FS mkdir; no hermetic spec surface (folder_test.go)",
	// run-re is the internal compiled-`re` consumer: it is reached only via the
	// re compile-hook splice (with a carrier users cannot construct), never by
	// name. Covered by lang/go/test/minilang_compile_test.go.
	"MiniLang.run-re": "internal compiled-re consumer; reached only via the compile-hook splice (minilang_compile_test.go)",
	// boru:model wraps github.com/voxgig/model, a build tool: model.New always
	// uses the real OS filesystem (reads .jsonic source, writes <base>/<name>.json)
	// and start/stop spin a background watch goroutine — none of which is a
	// hermetic, deterministic spec surface. Covered by lang/go/test/model_test.go.
	"Model.new":   "real-FS build tool; no hermetic spec surface (model_test.go)",
	"Model.run":   "real-FS build (reads/writes disk); no hermetic spec surface (model_test.go)",
	"Model.start": "background watch goroutine; not deterministic in a spec row (model_test.go)",
	"Model.stop":  "watch lifecycle; paired with start, no hermetic spec surface (model_test.go)",
	"Model.model": "real-FS build tool; no hermetic spec surface (model_test.go)",
	// Cli.main is the imperative shell over the pure Cli.dispatch
	// (design/CLI-PROGRAMS.1.md §2): every one of its arms ends in IO.exit,
	// and a SPEC ROW cannot survive that — the runner would end mid-file and
	// the rows after it would never run. (The module's own boru:test suite CAN
	// exercise it, via Assert.throws, and does — cli.boru is at 100% boru-line
	// coverage with an empty allowlist. What is missing here is a hermetic
	// spec surface, not a test.) The decision each arm acts on is pinned by
	// the Cli.dispatch rows in lang/spec/module-cli.tsv §4, and the shell is
	// driven end to end by cmd/go/internal/build/utils_e2e_test.go.
	"Cli.main": "every arm ends in IO.exit; a spec row cannot survive it (cli_test.boru covers it via Assert.throws; utils_e2e_test.go drives it end to end)",
	// Test.cover / Test.coverage measure INTERPRETER line coverage of a module-
	// under-test: the recorded rows (and thus the report) exist only on the
	// tree-walk path (the compiled VM never fires the coverage hook), so a spec
	// row would diverge compiled-vs-interpreter. Covered by the Go tests in
	// lang/go/modules/test_coverage_test.go + test_coverage_cov_test.go.
	"Test.cover":    "interpreter-only coverage measurement; mode-dependent, no hermetic spec surface (test_coverage_test.go)",
	"Test.coverage": "interpreter-only coverage report; mode-dependent, no hermetic spec surface (test_coverage_test.go)",
}

// TestModuleExportCoverage fails when any native-module export lacks a
// lang/spec row mentioning its qualified `Namespace.word` name.
func TestModuleExportCoverage(t *testing.T) {
	t.Parallel()
	specDir := filepath.Join("..", "..", "..", "lang", "spec")

	// Slurp every spec file once into a single haystack — the flat corpus
	// plus lang/spec/frontier/*.tsv: frontier rows are ordinary spec rows
	// run by the interpreter oracle (TestFrontierSpecInterp) with their
	// compile status ledgered, so a word whose result cannot yet be
	// def-bound in a compiling row (the fn-util family) is still genuinely
	// specced by its frontier rows.
	entries, err := specEntries(specDir)
	if err != nil {
		t.Fatalf("read spec dir %s: %v", specDir, err)
	}
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		paths = append(paths, filepath.Join(specDir, e.Name()))
	}
	frontierEntries, err := os.ReadDir(filepath.Join(specDir, "frontier"))
	if err != nil {
		t.Fatalf("read frontier spec dir: %v", err)
	}
	for _, e := range frontierEntries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		paths = append(paths, filepath.Join(specDir, "frontier", e.Name()))
	}
	var haystack strings.Builder
	for _, p := range paths {
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatalf("read %s: %v", p, rerr)
		}
		// Collect only the INPUT column (field 0) of real data rows.
		// Comment lines (leading '#') and the expected/description columns
		// are excluded so a comment mention can't fake coverage.
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
			input := line
			if tab := strings.IndexByte(line, '\t'); tab >= 0 {
				input = line[:tab]
			}
			haystack.WriteString(input)
			haystack.WriteByte('\n')
		}
	}
	specText := haystack.String()

	// Enumerate the live export set from the module registry.
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	// boru-implemented modules (report/test) parse their source via the
	// registry's ParseFunc — mirror lang.New's wiring so Resolve works.
	reg.SetParseFunc(parser.Parse)
	// boru:repl's boru preamble imports boru:net / boru:vm — wire the
	// native-module resolver as production does.
	modules.InstallResolver(reg)

	var qualified []string // every "Namespace.word" the modules export
	names := modules.Names()
	sort.Strings(names)
	for _, name := range names {
		desc, derr := modules.Resolve(name, reg)
		if derr != nil {
			t.Fatalf("resolve module %q: %v", name, derr)
		}
		// Exports is keyed by namespace (e.g. "ArrayUtil"); each value is
		// an OrderedMap of word -> FnDef.
		nss := make([]string, 0, len(desc.Exports))
		for ns := range desc.Exports {
			nss = append(nss, ns)
		}
		sort.Strings(nss)
		for _, ns := range nss {
			for _, word := range desc.Exports[ns].Keys() {
				qualified = append(qualified, ns+"."+word)
			}
		}
	}

	exportSet := make(map[string]bool, len(qualified))
	for _, q := range qualified {
		exportSet[q] = true
	}
	// A hermetic-exemption entry must name a live export, else it's stale.
	for q := range hermeticExempt {
		if !exportSet[q] {
			t.Errorf("hermeticExempt names %q, which is not (or no longer) a "+
				"native-module export — remove the stale exemption", q)
		}
	}

	var uncovered []string
	for _, q := range qualified {
		if hermeticExempt[q] != "" {
			continue // covered by Go tests; see hermeticExempt.
		}
		if !strings.Contains(specText, q) {
			uncovered = append(uncovered, q)
		}
	}

	if len(uncovered) > 0 && filteredCorpus() {
		t.Logf("%d export(s) unmentioned in the %d selected spec files (BORU_SPEC_FILES: a corpus-wide claim, not asserted)", len(uncovered), len(paths))
		return
	}
	if len(uncovered) > 0 {
		sort.Strings(uncovered)
		t.Fatalf("%d native-module export(s) have no lang/spec row "+
			"mentioning their qualified name (ADR-003). Add at least one "+
			"row per word to lang/spec/module-*.tsv:\n  %s",
			len(uncovered), strings.Join(uncovered, "\n  "))
	}
}
