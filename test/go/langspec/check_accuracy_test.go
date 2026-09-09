// Check-accuracy ratchet (design/checker-accuracy-review.10.md §5).
//
// Runs `boru check` semantics (Registry.Check.Begin + a normal engine
// run) over every row of the production language spec at lang/spec/
// and counts two things:
//
//   - FALSE POSITIVES: rows whose expectation is a VALUE (the program
//     is correct and runs) but where the checker reports an
//     error-severity diagnostic or a hard error. Target: 0.
//   - UNFLAGGED ERROR ROWS: rows whose expectation is ERROR:* but
//     where the checker is silent. This will never reach 0 — many
//     spec errors are value-dependent (overflow, missing keys,
//     division by zero) and are the runtime's job — so the count is
//     a trend metric, not a target. It is pinned PER SPEC FILE
//     (unflaggedPins), so a change touching one file edits one line.
//
// The pins are ratchets: a change that pushes a count above its pin is
// a checker-accuracy regression and fails this test. When a fix lowers
// a count, lower the pin in the same commit so the gain is locked in.
// The long historical rationale for each pin's value lives in
// design/CHECK-ACCURACY-RATCHET.10.md (moved out of inline comments so
// routine pin bumps stop producing 20 KB single-line merge conflicts).
package langspec

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/parser/go"
	"github.com/boru-lang/boru/test/specfix"
)

// pinnedFalsePositives is the whole-corpus count of VALUE rows the checker
// wrongly errors on. A ratchet held at zero: any rise is a checker regression.
// The historical rationale that used to live here inline moved to
// design/CHECK-ACCURACY-RATCHET.10.md (§ "False positives").
const pinnedFalsePositives = 0

// unflaggedPins is the PER-SPEC-FILE count of `ERROR:` rows the checker leaves
// silent — overwhelmingly runtime-only / value-dependent errors (malformed
// sources fed to registered parsers, division by zero, missing keys, builder
// and registration state) that are the runtime's job, not statically flaggable.
//
// It replaces a single global pin so a change touching one spec file edits ONE
// line here instead of a 20 KB shared line — two branches adding rows to
// different files no longer conflict. Ratchet rules, per entry:
//   - adding spec rows that raise a file's count: bump that file's entry and
//     say why in the commit message;
//   - a checker improvement that lowers a count: lower the entry to lock it in
//     (the test logs the suggestion);
//   - a file absent from this map is expected to have ZERO unflagged rows; the
//     first unflagged row in it fails the test until you add an entry.
//
// Keep entries sorted by filename so new files slot in predictably. The
// aggregate history is archived in design/CHECK-ACCURACY-RATCHET.10.md.
var unflaggedPins = map[string]int{
	// accessor.tsv: both unflagged rows are STORE misses (get + the NUR021
	// getr twin) — deliberately unproven: the context store is open-world
	// (a prototype layer / another scope may bind the key), so
	// getStoreReturnsFn stays optimistic on a static miss by design. The
	// Xml strict-miss row in the same batch IS flagged (getrXmlReturns).
	"accessor.tsv": 2,
	// bytecode-migrated.tsv: 0 → 1 on 2026-09-07 (the twenty-sixth
	// increment's graduation of frontier-hof-audit.tsv's §9 mk0 row): a
	// 0-arg apply of a 1-arg captured fn inside a returned lambda raises
	// signature_error at RUNTIME, when the closure is applied — the checker
	// sees a Function-typed capture and cannot know its arity statically.
	// (The twenty-seventh increment's graduated apply-over-a-gradual-lead
	// negative twin is NOT among these: the plain check flags its no-match
	// at the call over the concrete rule map.)
	"bytecode-migrated.tsv": 1,
	// fnpred.tsv: ONE unflagged class, and every unflagged row is in it — a
	// CONCRETE value failing a predicate (`def q:Even 5`, and the typed-param
	// twin `f 5`). Running a user predicate over a literal is decidable in
	// principle, but the check pass does not, so the membership failure
	// surfaces at runtime. record.tsv carries the same gap for the same
	// shape, and the capitalised-fn form `fnpred` replaces is unflagged
	// identically — so this is not a regression the word introduced.
	//
	// 6 -> 5, 2026-08-25: the pin and its comment were written from reasoning
	// rather than measurement when the word landed, and named two further
	// classes that the checker in fact DOES flag — the malformed spec lists
	// (`fnpred_invalid_spec`) and the unknown param type. Measured with
	// BORU_LOG_UNFLAGGED=1; only the five membership rows remain.
	"fnpred.tsv": 5,
	// fn-value.tsv: the §7 bare read of a Function param (NUR123) — the
	// checker binds a CARRIER for the param, which no signature can
	// dispatch, so the no-match a 1-arg lambda raises when read with no
	// argument is the runtime's (both lanes raise it; the interpreter's own
	// word dispatch under the binding name).
	"fn-value.tsv": 1,
	// as.tsv: the weak-payload guard row is a RUNTIME-only refusal by
	// design — the ascribed dispatch statically commits the base FlexMap
	// overload (sound: the interpreter takes the same widened match), and
	// the base handler's own payload-kind check (`set: expected a FlexMap,
	// got WeakFlexMap`) fires over the real runtime value; the check-mode
	// carrier has no payload to inspect. The second unflagged row is the
	// gradual §7 one: a dynamic(Any) value ascribed to Map passes the
	// check optimistically (the tree lattice cannot refute it) and the
	// as handler's runtime validation raises as_error over the concrete
	// Integer.
	"as.tsv":                2,
	"case.tsv":              2,
	"class.tsv":             1,
	"compare-restrict.tsv":  2,
	"control.tsv":           1,
	"edge-containers-2.tsv": 1,
	"edge-containers-3.tsv": 5,
	"edge-dispatch-2.tsv":   0,
	// 3 -> 6: the three string-option leniency rows became ERROR rows when
	// unknown option keys and out-of-domain values started being rejected.
	// They are RUNTIME rejections — the option map is a plain Map at a Map
	// slot, so the checker cannot see the key set statically — hence
	// unflagged rather than a checker regression.
	"edge-dispatch-3.tsv": 6,
	"edge-errors-1.tsv":   3,
	"edge-errors-2.tsv":   1,
	"edge-forward-1.tsv":  1,
	"edge-forward-2.tsv":  1,
	// edge-quote-1.tsv / edge-quote-3.tsv: one row each, the same shape —
	// a WORD element pulled out of a quoted list bare (`quote [add 1 2] get
	// 0`, and its macroexpand twin) re-fires at the pointer and raises
	// `signature_error` at run time. Newly unflagged 2026-08-26, and the
	// flag they lost was a PHANTOM: check mode's `get` yields a Word-typed
	// CARRIER, which the step loop used to dispatch as a token even though
	// a carrier has no WordInfo and therefore no name — so what "flagged"
	// these rows was `undefined_word` naming no word, at no position
	// (NUR103). With that arm fixed the checker types the row `Word`, which
	// is exactly right; whether the word it stands for dispatches cleanly
	// depends on WHICH word and is not decidable from the carrier.
	"edge-quote-1.tsv": 1,
	"edge-quote-3.tsv": 1,
	// edge-scalars-3.tsv: the pad byte-cap PROJECTION row (PR #306
	// review — a multi-byte fill exceeding maxStringResultBytes) is a
	// value-dependent resource bound, the runtime's job.
	"edge-scalars-3.tsv": 1,
	"edge-types-2.tsv":   3,
	"edge-types-3.tsv":   3,
	"error.tsv":          1,
	// flex.tsv: 9 -> 10 with the NUR022 `del` rows. The tenth is the Store
	// delete-then-read row, unflagged for exactly the reason accessor.tsv's
	// entry above gives — a context store is OPEN-WORLD, so a static miss
	// is never proven. delStoreReturnsFn widens the deleted key to dynamic
	// Any rather than recording it absent, because the shape model is
	// join-only monotone: "definitely gone" is a narrowing claim a later
	// set on another path would falsify. The five `del` REFUSAL rows in
	// the same batch (Class, Micron, List, FlexList, WeakFlexList) ARE all
	// flagged by their guaranteed-error mirrors.
	"flex.tsv":            10,
	"forward-barrier.tsv": 1,
	"generics-class.tsv":  3,
	"generics-fn.tsv":     1,
	"generics.tsv":        3,
	"higher-order.tsv":    4,
	"macro.tsv":           1,
	"micron.tsv":          0,
	// module-array.tsv: 0 → 6, all six from NUR030's fix. `group`'s keys
	// are Strings only, and the refusal is a RUNTIME check on each key's
	// type — it cannot be static, because the signature is `[TList]` /
	// `[TList TList]` and a List's ELEMENT types are not part of it. A
	// list whose elements are statically Integer is still a well-typed
	// argument; only walking it finds the bad key. Flagging these would
	// need element-type carriers on the list slots, which the checker
	// does not have and this record did not open.
	"module-array.tsv":    6,
	"module-debug.tsv":    3,
	"module-emitlang.tsv": 8,
	"module-fmt.tsv":      3,
	// module-fn.tsv: the two `FnUtil.curry` refusal rows. Both are the
	// native's own RUNTIME shape checks over the operand it received — a
	// multi-overload function, and a unary one — and the checker sees a
	// well-typed Function argument at a Function slot with nothing to prove
	// wrong: overload count and parameter count are payload facts, not
	// signature facts. (They graduated here from the frontier ledger on
	// 2026-08-25, when the fn-util family declared CompileStoresFn and the
	// Stage 3 fn-operand wall in front of them lifted; they were unflagged
	// there for the same reason.)
	"module-fn.tsv": 2,
	// 34 → 35: C2's read-line refuses an OUTPUT stream at runtime — stdout is
	// a perfectly good StreamKind, so the shape checks out statically and only
	// the handler knows it is the wrong direction. Its two sibling negatives
	// (a String where a stream is required, an Integer where a stream is
	// required) ARE shape refusals, so the checker flags them and they do not
	// move this count. That split is the rule: shape is static, value and
	// direction are not.
	//
	// 29 → 34: C1 added five ERROR: rows the STATIC checker cannot see.
	// boru/exit is raised by the exit handler at RUNTIME (IO.exit 3 / 0, and
	// the one crossing a handler-less `do`), and the 0..125 range check is
	// a runtime guard on a value the checker treats as an ordinary Integer
	// (IO.exit 126 / 200). The two rows the checker DOES flag — the String
	// code and the non-String env name — are signature errors, which is
	// exactly the line between the two: shape is static, value is not.
	"module-io.tsv":        30,
	"module-log.tsv":       6,
	"module-minilang.tsv":  20,
	"module-net.tsv":       2,
	"module-parse.tsv":     4,
	"module-parselang.tsv": 13,
	"module-query.tsv":     1,
	// module-rand.tsv: 0 -> 1. `Rand.list-of [] 2` — an EMPTY generator
	// body with a positive count. The emptiness is visible statically, but
	// whether it is an error depends on the COUNT, which is a plain Integer
	// slot: n=0 runs the body zero times and answers [], n>0 raises. A
	// value-dependent error the checker cannot decide without the value.
	// path-modifier.tsv: 1 -> 2, and BOTH rows are the same undecidable shape —
	// a division by a zero the checker sees only as an Integer. The second is
	// `def m {d:div/v} end 10 0 m.d/s`, added with the /s trailing-apply fast
	// lane to cover that lane's ERROR arm, which cover-gate had flagged as
	// unreached. Writing it is what exposed NUR113: the compiled lane answered
	// with the right error and NO caret where the island it replaced had one.
	// Both engines now report 1:26, so the row pins the position as well as
	// the taxonomy.
	//
	// path-modifier.tsv: the first row, `def m {d:div/v} end m.d 0 10` — a division
	// by zero reached through a parked native. The divisor is a plain Integer
	// literal, and whether dividing by it errors is a fact about its VALUE, not
	// its type; the checker types the row and stops there. Genuinely
	// undecidable statically — unlike the retired fn-value.tsv entry, which
	// pinned an error the checker merely declined to look for (NUR111).
	//
	// It is in the corpus for its DIAGNOSTIC, not its taxonomy: both engines
	// have always agreed this raises arith_error, and what the row pins is that
	// the compiled lane still points at 1:10. That is invisible to every gate
	// but the compile-or-fallback walk, which compares positions.
	//
	// path-modifier.tsv: 2 -> 3, and the third is the SAME undecidable shape
	// once more — `def m {d:div/v} end m.d/u 4 0`, a division by a zero the
	// checker sees only as an Integer. It exists for the same reason the /s row
	// above does: cover-gate flagged the USURP lane's error arm as unreached
	// once that lane stopped islanding, and the arm is only reachable through a
	// handler that actually raises. Three rows, one shape, three lanes — parked
	// native, trailing apply, usurp — which is the pattern to expect as each
	// remaining lane graduates off the island.
	"path-modifier.tsv": 3,
	"module-rand.tsv":   1,
	"module-sift.tsv":   24,
	"module-struct.tsv": 2,
	// module-test.tsv: 0 -> 3. The three check-prop count guards
	// (runs < 1, max-shrinks < 0) are RUNTIME value checks — the
	// signature slots are plain Integer, so the checker cannot see
	// the domain statically.
	"module-test.tsv":      3,
	"module-time.tsv":      2,
	"module-tui.tsv":       5,
	"module-vault-tui.tsv": 1,
	"module-vault.tsv":     43,
	"open-words.tsv":       3,
	"patrun.tsv":           1,
	"reach.tsv":            3,
	// record.tsv: 2 → 3 with NUR069's enforcement rows. Three ERROR rows
	// were added (a module fn returning the wrong type in each direction,
	// and a predicate's failing branch); the checker statically flags two
	// of them and misses one — the enforcement itself is a RUNTIME check
	// on the CallBoru dispatch path, so a violation the static pass
	// cannot resolve to a concrete return value stays the runtime's job.
	"record.tsv":            3,
	"scalar-micron-ops.tsv": 1,
	"storage.tsv":           1,
	"usurp.tsv":             1,
	"user-types.tsv":        1,
	// valof.tsv (was ref.tsv, pinned at 1): 1 → 2 with the /v totality
	// rows, then 2 → 0 with NUR073's BROAD park (2026-08-24). §2's
	// paren rows were rewritten from "the paren re-steps and fires" to
	// "the paren PLACES the held value", which retired the two
	// unbound-name ERROR rows the old spellings carried — nothing in
	// the file is left for the static pass to miss.
	"valof.tsv": 0,
	// The weak set/append refusals and typed weak writes are check-
	// mirrored (weakValueMirror + d2CheckWrite, native_storage.go). The
	// residue is make's own errors — source-family mismatch and a
	// refused CONSTRUCTION entry (no make mirror) — plus an
	// out-of-bounds index the static length tracker cannot see.
	"weak-flex.tsv": 4,
}

func TestCheckAccuracyRatchet(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var falsePositives, unflagged, valueRows, errorRows int
	unflaggedByFile := map[string]int{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		// bytecode-combinations.tsv is a compiled-vs-interpreter PARITY
		// fixture (validated by the differential / whole-corpus / spec
		// gates), not a checker-accuracy spec — its rows deliberately
		// exercise dynamic/island shapes the gradual checker widens, so
		// it must not move the false-positive / soundness baselines.
		if e.Name() == "bytecode-combinations.tsv" {
			continue
		}
		path := filepath.Join(specDir, e.Name())
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue // malformed rows are TestSpecProd's problem
			}
			input := strings.TrimSpace(parts[0])
			expected := strings.TrimSpace(parts[1])
			expectError := strings.HasPrefix(expected, "ERROR:")

			flagged := checkFlagsError(t, input)

			if expectError {
				errorRows++
				if !flagged {
					unflagged++
					unflaggedByFile[e.Name()]++
					if os.Getenv("BORU_LOG_UNFLAGGED") != "" {
						t.Logf("UNFLAGGED %s:L%d: %s", e.Name(), lineNum, input)
					}
				}
				continue
			}
			valueRows++
			if flagged {
				falsePositives++
				if os.Getenv("BORU_LOG_FALSEPOS") != "" {
					t.Logf("FALSEPOS %s: %s", e.Name(), strings.TrimSpace(parts[0]))
				}
				t.Logf("FALSE POSITIVE %s:L%d: %s", e.Name(), lineNum, input)
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error in %s: %v", path, err)
		}
	}

	t.Logf("check-accuracy: %d/%d value rows falsely flagged; %d/%d error rows unflagged",
		falsePositives, valueRows, unflagged, errorRows)

	if falsePositives > pinnedFalsePositives {
		t.Errorf("false positives rose to %d (pin %d) — the checker now wrongly rejects correct spec rows",
			falsePositives, pinnedFalsePositives)
	} else if falsePositives < pinnedFalsePositives {
		t.Logf("false positives improved to %d — lower pinnedFalsePositives to lock it in", falsePositives)
	}

	// Per-file unflagged comparison: a file above its pin is a REGRESSION
	// (lost coverage, or spec rows added without bumping its entry); a file
	// below its pin is an IMPROVEMENT to lock in. Localizing to the file keeps
	// each change editing one line of unflaggedPins.
	var over, under, unknown []string
	for name, got := range unflaggedByFile {
		pin, ok := unflaggedPins[name]
		switch {
		case !ok && got > 0:
			unknown = append(unknown, itoaKV(name, got))
		case got > pin:
			over = append(over, itoaKV(name, got)+" (pin "+itoa(pin)+")")
		case got < pin:
			under = append(under, itoaKV(name, got)+" (pin "+itoa(pin)+")")
		}
	}
	// A pin whose file no longer produces any unflagged row is fully stale.
	for name, pin := range unflaggedPins {
		if _, seen := unflaggedByFile[name]; !seen && pin > 0 {
			under = append(under, itoaKV(name, 0)+" (pin "+itoa(pin)+")")
		}
	}
	sort.Strings(over)
	sort.Strings(under)
	sort.Strings(unknown)

	if len(unknown) > 0 {
		t.Errorf("%d spec file(s) have unflagged ERROR rows but no unflaggedPins entry — "+
			"add an entry (these are runtime-only errors the checker cannot statically flag):\n  %s",
			len(unknown), strings.Join(unknown, "\n  "))
	}
	if len(over) > 0 {
		t.Errorf("%d spec file(s) regressed above their unflaggedPins entry — the checker lost "+
			"coverage, or spec rows were added without bumping the entry:\n  %s",
			len(over), strings.Join(over, "\n  "))
	}
	for _, u := range under {
		t.Logf("unflagged improved: %s — lower its unflaggedPins entry to lock it in", u)
	}
}

// itoaKV formats a "name=n" pair (itoa lives in compiled_fullcorpus_test.go).
func itoaKV(name string, n int) string { return name + "=" + itoa(n) }

// checkFlagsError runs one spec row in check mode against a fresh
// production registry (the same setup as runSpecProd) and reports
// whether the checker flags it: a parse failure, a hard run error, or
// any error-severity diagnostic.
func checkFlagsError(t *testing.T, input string) bool {
	t.Helper()
	values, err := parser.Parse(input)
	if err != nil {
		return true // parse errors are flagged by definition
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	specfix.RegisterQFixtures(reg)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)
	native.SetHostClock(reg, specClock)

	done := reg.Check.Begin()
	_, runErr := native.NewTop(reg).Run(values)
	// Mirror lang.(*Boru).Check: a fn-body forward reference to a name defined
	// LATER in the same program (mutual recursion isod/isev, a body referencing
	// a sibling def below it) is flagged undefined_word at eager body-analysis
	// time, then rescued once the def exists. The real checker drops these
	// before reporting; the ratchet must too, or it over-counts forward-ref
	// false positives the CLI never shows.
	reg.RescueForwardRefDiagnostics()
	diags := reg.Check.Diagnostics
	done()

	if runErr != nil {
		return true
	}
	for _, d := range diags {
		if d.Severity == core.SeverityError {
			return true
		}
	}
	return false
}

// ---- type-soundness differential (checker-accuracy-review.10.md §5,
// follow-on): for every value row that BOTH checks clean and runs,
// the runtime result's type must be covered by the checked carrier —
// `typeof(actual) ⊑ checked`. This is the assertion that catches
// wrong-TYPE checker bugs (A1, A4), which the value-pinning ratchet
// cannot see. Violations are pinned and may only decrease.

// pinnedTypeSoundnessViolations is the whole-corpus count of clean value rows
// whose runtime result type is NOT covered by the checked carrier (a wrong-TYPE
// checker bug the value-pinning ratchet can't see). Held at zero. History:
// design/CHECK-ACCURACY-RATCHET.10.md (§ "Type-soundness violations").
const pinnedTypeSoundnessViolations = 0

func TestCheckTypeSoundness(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var violations, compared int

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		// bytecode-combinations.tsv is a compiled-vs-interpreter PARITY
		// fixture (validated by the differential / whole-corpus / spec
		// gates), not a checker-accuracy spec — its rows deliberately
		// exercise dynamic/island shapes the gradual checker widens, so
		// it must not move the false-positive / soundness baselines.
		if e.Name() == "bytecode-combinations.tsv" {
			continue
		}
		path := filepath.Join(specDir, e.Name())
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			input := strings.TrimSpace(parts[0])
			expected := strings.TrimSpace(parts[1])
			if strings.HasPrefix(expected, "ERROR:") {
				continue
			}

			checked, flagged := checkRow(t, input)
			if flagged {
				continue // counted by the FP ratchet, not here
			}
			actual, ok := runRow(t, input)
			if !ok {
				continue // runtime-environment rows (fixtures etc.)
			}
			compared++
			if !stackTypeCovered(checked, actual) {
				violations++
				t.Logf("TYPE UNSOUND %s:L%d: %s\n  checked=%s actual=%s",
					e.Name(), lineNum, input, stackTypes(checked), stackTypes(actual))
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error in %s: %v", path, err)
		}
	}

	t.Logf("type-soundness: %d violations across %d compared rows", violations, compared)
	if violations > pinnedTypeSoundnessViolations {
		t.Errorf("type-soundness violations rose to %d (pin %d)", violations, pinnedTypeSoundnessViolations)
	} else if violations < pinnedTypeSoundnessViolations {
		t.Logf("violations improved to %d — lower pinnedTypeSoundnessViolations to lock it in", violations)
	}
}

// checkRow runs one row in check mode and returns the residual
// carrier stack plus whether the checker flagged it.
func checkRow(t *testing.T, input string) ([]core.Value, bool) {
	t.Helper()
	values, err := parser.Parse(input)
	if err != nil {
		return nil, true
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	specfix.RegisterQFixtures(reg)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)
	native.SetHostClock(reg, specClock)

	done := reg.Check.Begin()
	out, runErr := native.NewTop(reg).Run(values)
	diags := reg.Check.Diagnostics
	done()
	if runErr != nil {
		return nil, true
	}
	for _, d := range diags {
		if d.Severity == core.SeverityError {
			return nil, true
		}
	}
	return out, false
}

// runRow executes one row at runtime; ok=false when the row needs an
// environment this harness doesn't provide (it errored at runtime
// although the spec expects a value — fixtures, network, files).
func runRow(t *testing.T, input string) ([]core.Value, bool) {
	t.Helper()
	values, err := parser.Parse(input)
	if err != nil {
		return nil, false
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	specfix.RegisterQFixtures(reg)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)
	native.SetHostClock(reg, specClock)
	out, runErr := native.NewTop(reg).Run(values)
	if runErr != nil {
		return nil, false
	}
	return out, true
}

// stackTypeCovered checks the runtime stack against the checked
// carrier stack position-for-position from the TOP. A checked stack
// may be longer (None padding from branch joins); a runtime stack
// longer than the checked one is a violation.
func stackTypeCovered(checked, actual []core.Value) bool {
	// Variadic-spread bottom carrier (a `[]`-declared recursive fn whose depth
	// is a runtime value, e.g. recursion.tsv:53 → 0-or-more Integers): the
	// fixed prefix `checked[1:]` is checked top-aligned as usual, and the bottom
	// runtime entries beyond that prefix are absorbed — each must still pass the
	// real typeCovered(elem, ·), so this adds COUNT flexibility only, never TYPE
	// flexibility (a wrong-typed leak is still flagged).
	if len(checked) > 0 {
		if elem, ok := core.IsVariadicSpread(checked[0]); ok {
			fixed := checked[1:]
			if len(actual) < len(fixed) {
				return false
			}
			for i := 0; i < len(fixed); i++ {
				if !typeCovered(fixed[len(fixed)-1-i], actual[len(actual)-1-i]) {
					return false
				}
			}
			for i := 0; i < len(actual)-len(fixed); i++ {
				if !typeCovered(elem, actual[i]) {
					return false
				}
			}
			return true
		}
	}
	if len(actual) > len(checked) {
		return false
	}
	for i := 0; i < len(actual); i++ {
		c := checked[len(checked)-1-i]
		a := actual[len(actual)-1-i]
		if !typeCovered(c, a) {
			return false
		}
	}
	return true
}

// typeCovered reports whether one checked carrier admits the runtime
// value's type.
func typeCovered(checked, actual core.Value) bool {
	if checked.Dynamic {
		return true
	}
	// A type-as-value actual (typeof / type-algebra / `make`-of-a-type /
	// enum) is covered when its DENOTED node conforms to the checked node —
	// tested BEFORE the disjunct decomposition below. Without this, a checked
	// Enum/Disjunct value (whose DisjunctInfo toCarrier preserves) would be
	// split into its per-alternative atoms and matched against the whole
	// runtime enum value, which is nonsensical (corpus-core.tsv:61 — the two
	// enum mints share members and differ only by ID).
	if actualIsTypeValue(actual) {
		cnode := checked.Parent
		if core.IsBareTypeNode(checked) {
			cnode = core.ValueType(checked)
		}
		if cnode != nil && core.TType.ConformsTo(cnode) {
			return true
		}
		if dn := core.ValueType(actual); dn != nil && cnode != nil && dn.ConformsTo(cnode) {
			return true
		}
	}
	if core.IsDisjunct(checked) {
		di, err := core.AsDisjunct(checked)
		if err != nil {
			return false
		}
		for _, alt := range di.Alternatives {
			if typeCovered(alt, actual) {
				return true
			}
		}
		return false
	}
	node := checked.Parent
	if core.IsBareTypeNode(checked) {
		node = core.ValueType(checked)
	}
	if node == nil {
		return false
	}
	// The type-as-value case (typeof / type-algebra / make-of-a-type / enum)
	// is handled at the top of the function, before the disjunct branch.
	//
	// Tag-conformance OR runtime membership. For an ordinary carrier the
	// tag test suffices, but for a subset/singleton carrier the runtime value
	// keeps its BASE tag while genuinely inhabiting the checked type — the
	// checker's real promise is `runtime is checked`, not tag-subtyping. So a
	// predicate-refine (`Big` = Integer gt 10, whose Parent IS Integer, so a
	// concrete Integer's tag never conforms) and a const singleton (a member
	// subtype of ProperString) are covered via Value.Is, which runs the real
	// predicate / equality on the concrete value — still rejecting a genuine
	// non-member (user-types.tsv:124, class.tsv:85).
	return actual.Parent.ConformsTo(node) || actual.Is(node)
}

// actualIsTypeValue mirrors the runtime `is Type` membership rule
// (native_type.go isHandler): a value is itself a type when it is a
// bare type node, a structural type body, an (explicit-map) record
// shape, or a Type/-rooted value (Function / Disjunct / Enum). Used by
// typeCovered so a `typeof`/type-algebra result is judged by its
// type-membership, not by its denoted type's lattice parent.
func actualIsTypeValue(v core.Value) bool {
	return core.IsBareTypeNode(v) || core.IsTypeBody(v) ||
		core.IsRecordShape(v) || v.Parent.ConformsTo(core.TType)
}

func stackTypes(vs []core.Value) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = v.Parent.Leaf()
		if v.Dynamic {
			parts[i] = "dynamic(" + parts[i] + ")"
		}
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// ---- Any-frontier metric (the "narrow types instead of Any" goal):
// counts clean value rows whose residual carrier stack still contains an
// Any-bounded carrier — strict Carry<Any> or dynamic(Any), the two shapes
// where the checker gave up to Any instead of a narrow type. (A dynamic
// carrier with a real bound — dynamic(Integer) — is NOT counted; it has a
// useful type.) Pinned as a ratchet (only decreases) so return-annotation
// / ReturnsFn narrowing work is measured and an Any-widening regression is
// caught. Unlike type-soundness this is a PRECISION metric, not a
// soundness one: a high count is imprecise, not unsound.

// The Any-frontier COUNT is informational (logged, not pinned — pinning it churned
// on every corpus addition); only the RATIO is gated (anyFrontierRatioCeilingPct).
// Frontier-count history: design/CHECK-ACCURACY-RATCHET.10.md.

func TestCheckAnyFrontier(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var anyRows, valueRows int
	byFile := map[string]int{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		if e.Name() == "bytecode-combinations.tsv" {
			continue
		}
		f, err := os.Open(filepath.Join(specDir, e.Name()))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 || strings.HasPrefix(strings.TrimSpace(parts[1]), "ERROR:") {
				continue
			}
			checked, flagged := checkRow(t, strings.TrimSpace(parts[0]))
			if flagged {
				continue
			}
			valueRows++
			if residualHasAnyFrontier(checked) {
				if os.Getenv("BORU_LOG_ANYFRONTIER") != "" {
					t.Logf("ANYFRONTIER %s: %s", e.Name(), strings.TrimSpace(parts[0]))
				}
				anyRows++
				byFile[e.Name()]++
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner: %v", err)
		}
	}

	t.Logf("any-frontier: %d/%d clean value rows end with an Any carrier", anyRows, valueRows)
	for _, kv := range topFiles(byFile, 10) {
		t.Logf("  %3d  %s", kv.n, kv.name)
	}
	// The Any-frontier COUNT is informational (pinning it turns every corpus
	// addition into pin churn), but its RATIO is gated: the fraction of clean
	// rows that end at an Any carrier must not grow past the ceiling below.
	// New corpus rows move both numerator and denominator, so ordinary corpus
	// growth passes; only a systemic precision REGRESSION (a fix that widens
	// results to Any across the board) trips it. Lower the ceiling as
	// precision fronts land; never raise it without a documented decision.
	t.Logf("any-frontier %d/%d — count informational, ratio gated", anyRows, valueRows)
	// anyFrontierRatioCeilingPct gates the FRACTION of clean rows ending at an Any
	// carrier — corpus growth moves numerator and denominator together, so only a
	// systemic precision regression trips it. Lower as precision fronts land; never
	// raise without a decision. History: design/CHECK-ACCURACY-RATCHET.10.md.
	const anyFrontierRatioCeilingPct = 7
	if valueRows > 0 && anyRows*100 > valueRows*anyFrontierRatioCeilingPct {
		t.Errorf("any-frontier ratio %d/%d (%.1f%%) exceeds the %d%% ceiling — a systemic "+
			"precision regression widened results to Any; find the change that grew the "+
			"frontier instead of raising the ceiling",
			anyRows, valueRows, float64(anyRows)*100/float64(valueRows), anyFrontierRatioCeilingPct)
	}
}

// residualHasAnyFrontier reports whether any carrier in a residual stack
// is Any-bounded (strict Any carrier or dynamic(Any)) — the "gave up to
// Any" shape the frontier metric counts.
func residualHasAnyFrontier(stk []core.Value) bool {
	for _, v := range stk {
		if v.Parent != nil && v.Parent.Equal(core.TAny) {
			return true
		}
	}
	return false
}

type fileCount struct {
	name string
	n    int
}

// topFiles returns the n spec files with the most Any-frontier rows,
// most first — a worklist of where narrowing would pay off most.
func topFiles(byFile map[string]int, n int) []fileCount {
	out := make([]fileCount, 0, len(byFile))
	for name, c := range byFile {
		out = append(out, fileCount{name, c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].n != out[j].n {
			return out[i].n > out[j].n
		}
		return out[i].name < out[j].name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
