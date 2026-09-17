// Compiled-mode differential gate (design/legacy/boru-bytecode-plan.0.ignore,
// ground rule "differential gate from day one"): every spec value
// row the Stage-1 emitter accepts must produce IDENTICAL results
// through the bytecode VM and the interpreter. The compiled-row
// count is pinned with a floor so a regression that silently refuses
// everything (vacuously passing the equality check) is caught.
//
// See design/COMPILABLE-SUBSET.md for the positive statement of what this gate
// is guarding — the rule each emitter refusal is defending.
package langspec

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// newDifferentialInstance builds one side of the comparison with the
// SAME frozen clock the spec runner uses (langspec_test.go), so
// clock-seeded rows (rand, now) are deterministic and comparable
// across the two runs.
func newDifferentialInstance(t *testing.T) *lang.Boru {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	a.SetClock(specClock)
	return a
}

// minCompiledRows is the floor: at least this many spec rows must
// take the compiled path for the gate to be meaningful. Raise it as
// later stages widen the compilable subset; never lower it without a
// documented decision.
const minCompiledRows = 6410 // + F4 class/object make (plain-data const-bake) + get islands + fn-value boundary + is/typeof on make-result (type-operand ID-collision guard): 1636 compiled June 2026; + P5 multi/0-result calls: 1677 compiled; + multi-return / 0-return / anonymous-lambda fns (N-result fn units): 1742; + apply of a fn value (check-engine re-step): 1753; + unnamed-fn map/list members (method fields m.f): 1759; + 0-value-then if statement guard: 1760; + concrete-args dynamic-output core builtins (unify et al): 1765; + module inner natives (dot-access): 1813; + 3-arg operand shape (sig-0 result over consts, push+swap chain): 1820; + strict-disjunct type-algebra poly (is/tand over tnot-predicate): 1825 (the differential count is sensitive to test grouping — this is the strict CI-group value; the full make-test grouping runs a few higher); + object/class atom-keyed set: 1839; + computed-else if (OpDrop): 1843; + variadic-else statement-if: 1847; + fn-value introspection (baked fn-value const): 1857  (map iteration moves the ISLAND ceiling 15->9, not the differential count — the islanded rows were already wasCompiled=true); + filter lambda args (list {key,value} + map KeyVal closures): 1861; + each/fold/scan map lambda args (KeyVal-shaped closure via the ClosureInShape flag): 1864; + with-decimal block (0-input body closure within the scoped decimal context): 1869; + args.N frame-local fold (get-N-of-args projection -> PUSH_LOCAL N): 1873; + word/splice inline expansion (__SP marker preserved through check mode, spliced at the use site): 1903; + macroexpand compile-time expansion baked as a code-as-data const (Word admitted as a const member): 1907; + make computed container defaults const-folded (class instance/scalar defaults + data-map values): 1920; + usurp (wrapper re-dispatch run in check mode -> reversed original call compiled): 1956; + nested-variadic branch lowering (no-default case chains): 1958; + query DSL (boru:query): NoEvalArgs clause words (select/where/order/...) bake inert clause-lists, quoted-atom words (from/join/innerjoin/leftjoin/crossjoin) bake inert table atoms via the module-inner quote exemption: 1983; + reach lenses (the receiverless inert lens `$.name` bakes as a const via isInertReach, unblocking the lens-value / typeof / apply / rebind / getpath / setpath forms; the each/filter/sortby lens forms stay refused on the orthogonal list-element carrier-strip limit): 1996; + scalar carrier-keep (concrete inert scalars stay concrete through check mode, so a string/bool/temporal-bearing data list or map bakes as an inert const — `each`/`sortby`/`size` over string data, faithful class-default type bodies) + carrier-identity de-collision (repeated identical computed calls mint distinct result ids): 2007; + OpMakeList (top-level computed list literals `[1 add 2]` assemble their evaluated core-builtin elements via a new VM opcode; cleared the last Vm.run(canon …) tier-1 row too): 2052; + factory-apply (a fn that RETURNS an anonymous capture-free closure compiles to OpPushClosure; a [Function]-typed carrier leading the residual applies via a stack OpCallDynamic — `(mk2 5) 10` -> 11) and accumulated prior gains: 2195; + value-def-locals (named `def` bindings promote to frame locals, re-pushable in any order) + class mutable-default bake (flex/Array/Store/Object schema defaults bake as a const template make freshens per instance): 2200; + merge of origin/main (PR #151 concat-overload tightening + new spec rows): 2204; + branch-fragment value-def locals (enclosing computed value read inside a branch/clause fragment promotes to a frame local, unblocking computed-scrutinee `case`): 2210; + fn-value field calls (a baked map/object fn-value field applies faithfully via get-fold + CALL_DYNAMIC, with a trivial-delegation guard routing user fns through the island): 2220; + fn-body return-count (a 0-output guard's phantom None excluded from the count + outOps, so a guard-plus-result body compiles): 2222; + codequote code-as-data (a Quoted ParenExpr bakes as an inert const): 2224; + WS4 priority-1 migration (41 off-corpus Go parity regressions -> lang/spec/bytecode-migrated.tsv, all native): 2265; + the twin regime becomes the ONLY regime (§6.5's flip — this lane inherits the regime lane's own floor, 6410, measured at 6416 the day it landed): 6410

func TestSpecCompiledDifferential(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}

	var compiled, mismatches int

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
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
			if strings.HasPrefix(strings.TrimSpace(parts[1]), "ERROR:") {
				continue
			}

			// Compiled path on a fresh instance.
			ac := newDifferentialInstance(t)
			gotC, wasCompiled, errC := ac.RunCompiled(input)
			if !wasCompiled {
				continue // beyond the current stage's subset
			}
			compiled++

			// Interpreter path on another fresh instance.
			ai := newDifferentialInstance(t)
			gotI, errI := ai.RunInterp(input)

			if (errC != nil) != (errI != nil) {
				mismatches++
				t.Errorf("%s:L%d: %s\n  error divergence: compiled=%v interpreted=%v",
					e.Name(), lineNum, input, errC, errI)
				continue
			}
			if errC != nil {
				continue
			}
			if renderAny(gotC) != renderAny(gotI) {
				mismatches++
				t.Errorf("%s:L%d: %s\n  compiled=%q interpreted=%q",
					e.Name(), lineNum, input, renderAny(gotC), renderAny(gotI))
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error in %s: %v", path, err)
		}
	}

	t.Logf("compiled differential: %d rows compiled, %d mismatches", compiled, mismatches)
	if compiled < minCompiledRows {
		t.Errorf("only %d rows took the compiled path (floor %d) — the emitter regressed to refusing the corpus",
			compiled, minCompiledRows)
	}
}

func renderAny(vs []any) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, " ")
}
