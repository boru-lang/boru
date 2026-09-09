package langspec

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/parser/go"
	"github.com/boru-lang/boru/test/specfix"
)

// The shared frontier TSV corpus (lang/spec/frontier/*.tsv — the flat corpus
// glob skips subdirectories, so these rows sit OUTSIDE the live refusal/
// island ratchets). Each row is an ordinary spec row — `input⇥expected` with
// the interpreter as the semantics oracle, exactly the format a TS port will
// run — whose COMPILE status is the frontier: the expected-red ledger below
// pins which rows the compiler refuses today and why, with the same
// stale/drift/bootstrap contract as the lang-package frontier ledger
// (lang/go/frontier_ledger_test.go) and knownRefusals. Graduation = the row
// compiles → delete its ledger entry and (usually) move the row into the
// main lang/spec corpus so the census owns it.
//
// TestFrontierRefusalRowsCompile is the sibling inventory for the 9
// knownRefusals rows: those already live in the MAIN corpus, so their
// frontier cases read the sources straight from the knownRefusals map (one
// source of truth) and assert the TARGET (compile + byte-identical
// error/value parity); graduation is coupled per-row with the knownRefusals
// deletion.

type frontierRow struct {
	file  string
	line  int
	input string
	want  string
}

func loadFrontierRows(t *testing.T) []frontierRow {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "lang", "spec", "frontier")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("frontier spec dir: %v", err)
	}
	var rows []frontierRow
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("open %s: %v", e.Name(), err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				t.Fatalf("%s:%d: malformed row (no tab)", e.Name(), lineNo)
			}
			rows = append(rows, frontierRow{
				file:  e.Name(),
				line:  lineNo,
				input: strings.TrimSpace(parts[0]),
				want:  strings.TrimSpace(parts[1]),
			})
		}
		f.Close()
	}
	if len(rows) == 0 {
		t.Fatal("no frontier rows loaded")
	}
	return rows
}

// runFrontierInterp evaluates a row on the interpreter with the SAME wiring
// as the production spec runner (langspec_test.go's closure) and reports the
// canonical outcome string (core.Canon of the stack, or ERROR:<text>).
func runFrontierInterp(input string) (string, error) {
	values, err := parser.Parse(input)
	if err != nil {
		return "ERROR:" + err.Error(), nil
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		return "", err
	}
	specfix.RegisterQFixtures(reg)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)
	native.SetHostClock(reg, specClock)
	out, rerr := native.NewTop(reg).Run(values)
	if rerr != nil {
		return "ERROR:" + rerr.Error(), nil
	}
	return core.Canon(out), nil
}

// TestFrontierSpecInterp — the semantics oracle: every frontier row must PASS
// on the interpreter (rows are green semantics whose compile status is red).
// An expected of BOOTSTRAP fails printing the observed outcome verbatim, so
// populating a new row's expected column is enforced, exactly like the
// ledger's failsWith sentinel.
func TestFrontierSpecInterp(t *testing.T) {
	for _, row := range loadFrontierRows(t) {
		got, err := runFrontierInterp(row.input)
		if err != nil {
			t.Errorf("%s:%d: harness: %v", row.file, row.line, err)
			continue
		}
		switch {
		case row.want == "BOOTSTRAP":
			t.Errorf("%s:%d: BOOTSTRAP row — record the interpreter outcome into the expected column. Observed: %s", row.file, row.line, got)
		case strings.HasPrefix(row.want, "ERROR:"):
			if !strings.HasPrefix(got, "ERROR:") || !strings.Contains(got, strings.TrimPrefix(row.want, "ERROR:")) {
				t.Errorf("%s:%d: interpreter outcome %q, want error containing %q", row.file, row.line, got, row.want)
			}
		case got != row.want:
			t.Errorf("%s:%d: interpreter outcome %q, want %q", row.file, row.line, got, row.want)
		}
	}
}

// docMod is the shared module preamble of the do-catch rows (a value-
// dependently-raising fn and an always-raising one, reached as M.dec/M.boom).
// Must match the TSV rows byte-for-byte — the orphan arm catches drift.
const docMod = `import module [ def dec fn [[bad:Boolean x:Any] [Any] [ if bad [raise bad_input "boom"] [x] ]] def boom fn [[x:Any] [Any] [ raise bad_input "always" ]] export "M" {dec: dec/v, boom: boom/v} ] end `

// hof* — shared def prefixes of the frontier-hof-audit.tsv rows (the
// higher-order audit's §1 programs, design/HIGHER-ORDER-FUNCTIONS.0.md).
// Must match the TSV rows byte-for-byte — the orphan arm catches drift.
const (
	hofSKI    = `def kk x:Any => [y:Any => [x]] end def ss f:Function => [g:Function => [x:Any => [(f x) (g x)]]] end def ii ((ss kk/v) kk/v) end `
	hofBB     = `def bb f:Function => [g:Function => [x:Any => [f (g x)]]] end `
	hofNum    = `def czero f:Function => [x:Any => [x]] end def csucc n:Function => [f:Function => [x:Any => [f ((n f/v) x)]]] end `
	hofNumEnv = `def toint n:Function => [((n (k:Integer => [add k 1])) 0)] end def c1 (csucc czero/v) end def c2 (csucc c1/v) end def c3 (csucc c2/v) end `
	hofBool   = `def ctrue t:Any => [f:Any => [t/v]] end def cfalse t:Any => [f:Any => [f/v]] end def cif p:Function => [t:Any => [e:Any => [((p t) e)]]] end `
	hofFactk  = `def factk fn [[n:Integer k:Function][Any][ if (lte 1 n) [ (k 1) ] [ def kk ( fn r:Integer Any [ def m (mul n r) (k m) ] ) (factk (sub 1 n) kk/v) ] ]] end `
	hofPitem  = `def pitem s:String => [ if (eq 0 (size s)) [ {ok:false rest:s val:None} ] [ {ok:true val:(slice 0 1 s) rest:(slice 1 (size s) s)} ] ] end def psat fn p:Function Function [ ( fn s:String Map [ def r (pitem s) if (r.ok) [ if (p (r.val)) [ r ] [ {ok:false rest:s val:None} ] ] [ r ] ] ) ] end `
	hofPalt   = hofPitem + `def palt fn [[a:Function b:Function][Function][ ( fn s:String Map [ def r (a s) if (r.ok) [ r ] [ (b s) ] ] ) ]] end def ab (palt (psat (c:String => [eq 'a' c])) (psat (c:String => [eq 'b' c]))) end `
)

// frontierCompileLedger pins the frontier rows the compiler REFUSES today,
// keyed by exact input (the knownRefusals convention). failsWith pins the
// refusal reason substring (stable core only); "" is the bootstrap sentinel.
// Signatures transcribed from the 2026-07-13 bootstrap run.
var frontierCompileLedger = map[string]frontierEntryLS{
	// NUR098's fix (2026-08-24): apply is stack-only in BOTH overloads, so
	// the forward-lens spellings are check-time no-matches BY DESIGN
	// (frontier-apply-stackonly.tsv). The no-match takes the checker's
	// best-fit recovery, and a recovered dispatch refuses compilation
	// rather than compiling the error-trap program the main corpus's
	// refusal ceiling (0) requires. Graduation = the unmatched-dispatch
	// trap accepting a RECOVERED window; the rows then move to
	// lang/spec/apply.tsv §4's negatives.
	// (NUR099's `fnpred` `is` rows were ledgered here and have GRADUATED —
	// 2026-08-28, lang/spec/fnpred.tsv §7. The ledger's stated cause, "a
	// predicate type's constraint IS a function value, so `is` walks that
	// value into the Stage 3 gate", was wrong: a predicate NODE is a bare
	// type literal riding as data, and the body runs through the callback
	// seam, never the tape. The real blocker was that the seam declined
	// every detached unit mid-run, so the rows would have compiled with an
	// interpreter island hidden inside the handler. Closed by
	// eng/go/vm_foreign_unit.go.)
	// (stampFnConst's container descent briefly added TWO buckets here on
	// 2026-08-28 — a mount-handler seed refusing with "dynamic-scope def
	// `files` of unpromoted computed value" and its loop twin with "module
	// binding files rebound after a stored handler captured it as a dep".
	// Both were ONE leak: compileStoredFnUnit analyses against the LIVE emit
	// state, so the enclosing compile inherited a promotion decision and a dep
	// record from a body it may never apply. They were ledgered on the grounds
	// that the loop seed otherwise MISCOMPILES — and that was the mistake: the
	// descent masked the miscompile rather than fixing it, and the mask lifts
	// silently whenever the stamp declines. The stamp now restores
	// es.dynScopeNames and an optional ref no longer escalates a rebind, so
	// both rows compile and the divergence is pinned where divergences belong,
	// in varyKnownMiscompiles.)
	`def p {name:'ada'}  p apply $.name`: {why: "forward-lens no-match takes dispatch recovery (apply is stack-only, NUR098's fix); graduation = trap for a recovered window", failsWith: "unmatched dispatch recovered at apply"},
	`[10 20 30] apply $.1`:               {why: "forward-lens no-match takes dispatch recovery (apply is stack-only, NUR098's fix); graduation = trap for a recovered window", failsWith: "unmatched dispatch recovered at apply"},
	// (ADR-016 / NUR077 §5 Hole 1 was ledgered here and has GRADUATED —
	// `def f ([] => [42])  f/v apply` compiles with parity. The refusal was
	// an artefact of applying at the handler: that left the check engine's
	// re-step to const-fold the program to the FUNCTION. Marking the value
	// instead (FnDefInfo.Applied) puts both engines on the one gate, so the
	// check pass models the applied result and there is nothing to refuse.
	// Pinned in lang/spec/valof.tsv §5.)
	// NUR054 — a `context` read inside an inline-lowered body (here an
	// auto-evaluated def-list): the interpreter gives that body its own
	// context layer, the inline stream has no layer to hand out, and every
	// layer-distinguishing consumption of the handle would diverge — so the
	// mint refuses (recordDispatchOutcome) and the interpreter owns the
	// program. Moved from flex.tsv:304 (its point is flex gradual typing,
	// not context scoping — the green semantics stay on record here).
	// Graduation = an emitted context-frame opcode pair bracketing
	// inline-lowered regions.
	`context set 'k' 1 end context del 'k' end context set 'k' 2 end def l [(context get 'k')] (l get 0) add 1`: {why: "NUR054: a context read inside an auto-evaluated list has no compiled context layer", failsWith: "no layer to hand out"},

	// Conditional fn-shadow — a MISCOMPILE (variation sweep,
	// forward-barrier.tsv:73); now a SOUND REFUSAL: a user fn redefined
	// inside a conditionally-reached body overlap-removes the enclosing
	// overload in place, so the branch/loop def rollback cannot restore it and
	// compiled resolution bakes the shadow while the interpreter keeps the
	// outer fn on the not-taken / zero-iteration path. Refused CondBodyDepth-
	// gated (eng/go/core_helpers.go). Full graduation = a runtime dispatch
	// respecting the conditional binding compiles these rows.
	`def g fn [[x:Any] [Integer] [x add 100]] if false [def g fn [[x:Any] [Integer] [x add 1]]] g 1`: {why: "conditional fn redefinition shadows an outer overload; compiled bake would diverge from the interpreter on the not-taken branch", failsWith: "redefined inside a conditional body"},
	// The FN-BODY twin (the thirty-first increment): a capturing fn value —
	// a factory's returned closure — redefining an outer overloading def
	// from inside a fn body outlives the call on the interpreter (the
	// drop-then-push leaves the frame's def depth unchanged, so DefCleanup
	// pops nothing) where the compiled program kept the outer closure's
	// bake: `g (p 3)` answered `1 10` for the interpreter's `1 12`, on the
	// default lane, before the refusal. A capture-free literal takes the
	// compiled twin and agrees. Graduation = a runtime binding for the
	// closure's payload that the outer read sees after the call.
	`def kk k:Integer => [z:Integer => [add k z]] end def p (kk 7) end def g fn [[][Integer][def p (kk 9) 1]] end g (p 3)`: {why: "a fn body's def of a capturing fn value replaces an outer overloading def for good on the interpreter (§6.5's drop-then-push under the frame's cleanup); the compiled bake of the outer read would diverge (1 10 for 1 12 before the refusal)", failsWith: "redefined inside a fn body"},

	// audit §5.8: Church and (T and F) and or (T or F) — the thirty-second
	// increment lifted their refusal (a fn value beneath the apply word is
	// data), and they compile and agree, but a def'd lambda's VALUE (the main
	// program's `cfalse/v`; `ctrue/v` too in the or row) applied inside a
	// unit through a carrier lead takes the interpreter island, twice (the
	// island's interpreter-minted result islands again); their twins (T and
	// T, F or F) run native and graduated. Not the inner lambda's captures
	// (a capturing cfalse islands the same way). Graduation = the arriving
	// value carries a unit the op can enter.
	`def ctrue t:Any => [f:Any => [t/v]] end def cfalse t:Any => [f:Any => [f/v]] end def cif p:Function => [t:Any => [e:Any => [e/v (t/v p/v apply) apply]]] end def cand p:Function => [q:Function => [cfalse/v (q/v p/v apply) apply]] end 'F' ('T' (cif (cfalse/v (cand ctrue/v) apply)) apply) apply`: {why: "audit §5.8: Church and, T and F — compiles and agrees; cfalse's value applied inside cif's unit through the carrier lead islands (the thirty-second increment)", failsWith: "islanded"},
	`def ctrue t:Any => [f:Any => [t/v]] end def cfalse t:Any => [f:Any => [f/v]] end def cif p:Function => [t:Any => [e:Any => [e/v (t/v p/v apply) apply]]] end def cor p:Function => [q:Function => [q/v (ctrue/v p/v apply) apply]] end 'F' ('T' (cif (cfalse/v (cor ctrue/v) apply)) apply) apply`:    {why: "audit §5.8: Church or, T or F — compiles and agrees; the def'd lambda values applied inside cor's and cif's units through carrier leads island (the thirty-second increment)", failsWith: "islanded"},

	// L-DO — plan Phase 5: the body nets N values on no-raise but 1 Error on
	// raise; needs OpStackMark/OpDropToMark variable arity across the catch
	// merge. One entry per fallibility route (Reach raise, no-raise-at-input,
	// always-raise, value-diverging native, user fn body, bare module-export
	// value, branch-arm nesting).
	// L-DO PART 1 LANDED (2026-07-13): fallible multi-value do results now
	// record VARIADIC (the SetCatchVariadic latch) instead of refusing at the
	// ReturnsFn — the div row graduated to the main corpus, and the remaining
	// rows drifted to the DOWNSTREAM refusals below: `error` consuming the
	// variadic region's top needs the part-2 region-top lowering
	// (strip-input over a variadic region; see the L-DO implementation map
	// in the completion plan).
	// Re-diagnosed 2026-07-20 (PR #280 review): the def-bound variadic
	// region now refuses at lowerCall's store-prologue gate — a promoted
	// variadic result's stores pop success-arity values the raise path
	// never delivers — one stage before the "residual shape beyond Stage 1"
	// decline these rows used to surface. Same sound refusal, earlier and
	// truer diagnosis.
	// Re-diagnosed 2026-07-30 (design/FN-VALUE-DISPATCH.0.md): the region's
	// `M.dec` call fails dispatch, which is now an error-severity check
	// diagnostic in the model-undermining class (dispatch did not resolve, so
	// there is no call to compile) — the pipeline therefore refuses on the
	// diagnostics one stage before the promotion gate these rows used to
	// reach. The INTERPRETER runs them: the failure raises at the call, inside
	// the `do`, so the region's own handler catches it (see the note in
	// frontier-do-catch.tsv, and the check-vs-run divergence recorded in the
	// design note's §6). The L-DO promotion work below is still what would
	// graduate the SHAPE; these two rows can no longer witness it.
	docMod + `def msg (do [(true 5 M.dec) "no-raise"] error [dot code])  msg`:  {why: "plan Phase 5 (L-DO part 2): variadic region under a def binding — now check-rejected first (failed fn-value dispatch)", failsWith: "check diagnostics"},
	docMod + `def msg (do [(false 5 M.dec) "no-raise"] error [dot code])  msg`: {why: "plan Phase 5 (L-DO part 2): same shape, no raise at this input — likewise check-rejected first", failsWith: "check diagnostics"},
	// PR #280 review's promotion-gate representative (the variation
	// differential's prefix-stack find): a BRANCH-VARIANT multi-out do body
	// (0-or-2 values per arm) is variadic without any raise in sight, and a
	// dirty-stack prefix forces its promotion — the same exact-arity seat
	// hazard as the fallible regions.
	`7 def b true  do [1 2 (if b [] [9 9])]`: {why: "PR #280 review: branch-variant multi-out do region promoted under a dirty-stack prefix", failsWith: "variadic result promoted to frame slots"},

	// Chained forward application of Function params (frontier-chained-apply
	// .tsv) — the compose family, a live MISCOMPILE until 2026-08-02 (the
	// whole-frame replay's flat window lost the paren structure: compiled
	// RET count-error, interpreted 14), then a sound refusal
	// (noteDynFrameReplay declines a window with >1 applicable value).
	// GRADUATED 2026-08-03 in three coordinated steps: (1) the Stage-G
	// single-arg increment — a leading one-arg fn-carrier apply `(g x)`
	// records the trailing spelling's RecordDynApply event (compose/twice →
	// fn-value.tsv §8); (2) checkModeParenFnCollapse — the plain-surface
	// collapse twin — killed the def-split checker FP
	// (check_fn_param_apply_def_fp_test.go is the positive pin); (3) the
	// replayIsBodyTail windowReadsID widening — a dyn-bind of a value the
	// window reads is not a reorderable event — armed the def-split body
	// tail, so the stage row compiles natively too (fn-value.tsv §8). The
	// family's remaining refusal is the CHAINED MULTI-ARG apply
	// (`f (g x y)`), ledgered above in the emit-refusal families via
	// lang/go/bytecode_chained_apply_test.go's TestMultiArgChainedApplyRefuses
	// (no separate frontier entry: the two-applicable window refusal is the
	// §9.1 class, "unapplied fn-value in body residual").

	// Full-stack words GRADUATED 2026-08-03 (EmitState.FoldFullStack —
	// static fold over a provably-exact stack; rows moved to
	// corpus-core.tsv). The remaining sub-frontier
	// (frontier-full-stack.tsv): a roll permuting two EVENT results asks
	// the program residual to re-push call results in a non-production
	// order — beyond the Stage-1 residual discipline. The fold models it
	// correctly; the LOWERING declines. Graduation = program-residual
	// ordering beyond Stage 1.
	`(1 add 2) (3 add 4) 1 roll`: {why: "two event results permuted by roll: the residual re-push order exceeds Stage 1", failsWith: "residual shape beyond Stage 1"},
	// A full-stack word inside a wrapped code body compiles WITH an island
	// (the fold is gated to the top unit; the body's island machinery owns
	// the occurrence — sound interpreter re-entry, parity held). Graduation
	// = per-unit exactness for the fold. Same bucket pinned in
	// varyRefusalLedger ("islanded").
	`[10 20] each [drop 1 2 3 1 pick]`: {why: "full-stack word in a code body: the fold declines outside the top unit; the island seam owns it", failsWith: "islanded"},

	// Cross-module fn value in a higher-order word's CLOSURE slot
	// (design/FUNCTION-VALUE-SCOPE.0.md §12.3) — GRADUATED 2026-08-27
	// (Stage 3) into lang/spec/module-fnvalue-boundary.tsv §4.
	//
	// It was here because a fn value resolves its free words in its DEFINING
	// module while the closure lowering compiled the body against the CALLING
	// one, so compiling this baked the caller's `lim` (100) and returned []
	// where the interpreter returns [3 4] — a check-clean miscompile that
	// foreignFnHome declined into the callback seam's island.
	//
	// The graduation criterion written here — "compile the foreign body
	// against fd.Registry", with shareCheckStateFrom named as the CheckState
	// half — is exactly what landed, so this entry is worth reading as a
	// worked example of a ledger entry that paid off. Two details it got
	// right and one it got wrong:
	//
	//   RIGHT  the RUNTIME halves needed nothing (CompiledFn.Reg +
	//          enterUnit's curReg swap), and StartFnCompile's fnReg
	//          parameter already plumbed the compile side.
	//   RIGHT  CAPTURE OPERANDS are the unsolved role — and more unsolved
	//          than this entry knew. Looking a foreign body's module-scope
	//          mutable captures up in the CALLER (which is what the code
	//          did) compiles a closure over the WRONG cell whenever the two
	//          modules share a name: a SIXTH silent miscompile, in NUR101's
	//          family, found by writing the row. The lookup now uses
	//          fd.Registry, which declines at resolveOperand instead — the
	//          "registry-tagged dyn-scope operand" this entry named as the
	//          alternative is what would compile it. Fence:
	//          lang/go TestForeignClosureCaptureResolvesInItsOwnRegistry.
	//   WRONG  it read shareCheckStateFrom as one of several solved roles
	//          rather than as the whole remaining problem. A prototype that
	//          threaded fd.Registry and pointed only the foreign
	//          CheckState.Emit at the caller compiled a unit that const-
	//          folded the predicate to `false` — the body had no carrier for
	//          the per-element param, because the params live on the
	//          CheckState too. Sharing the WHOLE CheckState is the fix
	//          (check.ShareCheckStateFrom), and it is what makes the
	//          bindings-foreign / analysis-local split work.
	//
	// The context bracket noted here as a second asymmetry (enterBodyUnit
	// pushes/pops on the CALLING registry, vm.go:294-300, while curReg is
	// fd.Registry) did not need addressing for this row; it stays recorded
	// in case a future row reaches it.

	// Gradual-Any to a multi-overload user fn with DIFFERING arm returns —
	// the P1.3 target — GRADUATED 2026-08-03 (completeness-review §8.2(3)/
	// §9.11): tryCompileUserPolyArms records the position-wise JOIN of the
	// arms' returns (userPolyPlan.outs — a dynamic carrier at the arms'
	// common ancestor), userPolyArmShapeOK relaxed to count + nil-ness
	// agreement, and applyGradualContagion's first-match-partition widening
	// preserves the recorded identity (out[0].ID) so the poly event
	// survives to the elision. Both rows compile natively via
	// OpCallUserPoly (moved to lang/spec/fn-value.tsv §9; pinned in
	// lang/go/bytecode_poly_join_test.go with the count-mismatch negative).

	// `do … error` with a zero-netting handler (frontier-do-error-arity
	// .tsv) — the P1.6 target, GRADUATED 2026-08-03 for the PROVEN-raise
	// shape (completeness-review §9.13): a strict Error do-result fixes the
	// arity at zero, errorReturnsFn returns it truthfully, and the
	// strip-input shape screen's want-0 arm admits the empty residual —
	// the row compiles natively (moved to the main corpus; pinned in
	// lang/go/bytecode_do_error_arity_test.go). The MAYBE-raising twin
	// below keeps the refusal: a dynamic Error bound has variable arity
	// (pass-through 1 vs caught 0), the true remaining §8.2(6) target
	// (the variable-arity island via the mark machinery).
	`def xs [0] do [1 div (xs 0 getr)] error [drop] end 2 add 3`: {why: "maybe-raising body with a zero-netting handler: the pass-through nets one where the caught path nets zero — no fixed seat", failsWith: "handler nets no value"},

	// NUR038 statement-seal twin-call matrix (frontier-nur038-seal.tsv):
	// semantically green under the seal + arrival barrier; compile-refused
	// on the pre-existing residual-ordering limitation — two dynamic call
	// results in one program residual exceed the Stage-1 lowering. Sound
	// interpreter fallback. Graduation = multi-dynamic-result residual
	// lowering; the rows then move to lang/spec/fn-value.tsv §6.
	`def f fn [[x:Any] [Any] [x]] end def m {p: f/v} end 5 m.p m.p 7`:                   {why: "NUR038 seal: stack form then forward form", failsWith: "fn-value-call boundary"},
	`def f fn [[x:Any] [Any] [x]] end def m {p: f/v} end m.p (1 add 2) m.p 7`:           {why: "NUR038 seal: computed first argument", failsWith: "fn-value-call boundary"},
	`def m {l: ([x:Any] => [x])} end m.l 5 m.l 7`:                                       {why: "NUR038 seal: lambda twins", failsWith: "fn-value-call boundary"},
	`def e fn [[] [Integer] [42] [x:Any] [Any] [x]] end def m {e: e/v} end m.e 5 m.e 7`: {why: "NUR038 seal: mixed 0/1-arg overload twins (NUR035 guard)", failsWith: "fn value read from a container auto-dispatches"},

	// Namespace capture at a macro-expanded call site (the NUR038 wrapper
	// retirement's re-bucketed refusal — see frontier-capture-namespace.tsv):
	// an inner fn capturing a body-imported module namespace has no bakeable
	// operand home at the `parse` macro's expanded call site. Successor to
	// the graduated "closure captures a runtime-minted value" bucket (the
	// wrapper refused earlier, at capture-slot numbering). Full graduation =
	// a capture-slot lowering that materialises the namespace binding.
	`def zzvfn fn [[] [] [import "boru:parselang"  import "boru:string-util"  def calc (fn [[source:Any opts:Map] [List] [StringUtil.split ' ' (ParseLang.source source)]])  end  (parse calc {trace:true} 'x + y') get 1]] zzvfn`: {why: "inner fn captures the body-imported ParseLang namespace; no bakeable operand home at the macro-expanded call site", failsWith: "capture ParseLang of calc unreachable at a call site"},
	// GRADUATED 2026-07-17 (§9.1): the `do [M 3] error [dot code]` row
	// compiles — an identity-less dyn-body out (the module-export instance)
	// now mints a fresh ID at the record, restoring its tape placement and
	// event linkage, so the mark-window island owns the region as usual.
	// GRADUATED 2026-07-14 (the mark-window island, L-DO part 2b): the
	// error-over-the-variadic-region rows — do [(M.boom 5) "x"] / the [Any]
	// user-fn twin / the branch-arm nesting / the StructUtil.parse chained
	// leaf — compile natively: Finalize's markWindowShape opens an
	// OpStackMark before the region-starting do event and the residual
	// islands verbatim through OpCallDynMixedFromMark (rows moved to
	// lang/spec/bytecode-migrated.tsv; family pinned in
	// lang/go/bytecode_markwindow_test.go). The def-msg rows above and the
	// module-export row keep their sound refusals (a PROMOTED def read /
	// a non-event region entry decline the window).

	// The twin regime's placement frontier (frontier-twin-placement.tsv,
	// entered 2026-09-02 with the §6.5 default flip; the variation lane's
	// find, pinned there as the "twin regime (unplaced bind transition)"
	// bucket). Each row performs a bind transition inside a wrapped body
	// that no compiled op replays or installs — refused at Finalize's
	// full-placement gate rather than compiled on the check pass's kept
	// install, which is what the old default did. Sound; the interpreter
	// owns every row. Graduation per shape: resident module binds / type
	// twins inside compiled units, a closure lowering that admits the
	// declined do body, and the root cause of the import-and-call pair.
	`[10 20] each [drop import "boru:math-util" end MathUtil.cbrt 2]`:                       {why: "twin placement: an import inside a multi-run body is a module bind, not a BindDef the arm-residency bridge installs per element", failsWith: "no stream placement"},
	`[10 20] each [drop def A (Integer gt 10) def B (Integer lt 20) def x:(A tand B) 15 x]`: {why: "twin placement: a type def inside a multi-run body — the bridge pairs BindDef twins only", failsWith: "no stream placement"},
	`do [def b true  do [1 2 (if b [] [9 9])]]`:                                             {why: "twin placement: the do body's closure compile declines (Stage-3 residual shape), so the once-run body's def twin is never adopted", failsWith: "no stream placement"},
	// (The fourth shape — `do [import "boru:sift" (Sift.parse kv/q {} "a: 1")]`
	// — GRADUATED 2026-09-02 and its entry is deleted. It was the one this
	// ledger recorded as "measured, not yet root-caused", and the cause was
	// not what the entry guessed: the twin had a body site. The do's keep
	// bracket is a SINGLE LATCH, and tryRecordClosure's body compile RE-RUNS
	// the body, so a `do` inside a fn that the body CALLS — sift-parse-do's,
	// here — opened its own bracket during the re-run and overwrote the outer
	// one. The published range came back EMPTY ([1 1], floor already past the
	// import's own `Sift` twin), so AdoptBodyTwins had nothing to walk.
	// KeepDefsBodyGuard now publishes only at FnBodyDepth == 0, exactly as
	// its multi-run sibling does, and the row compiles with parity.)

	// NUR067 — await's winner-takes-all modes (frontier-await-winner.tsv):
	// `first` / `any` hand back the winning branch's WHOLE residual, 0-or-more
	// values — a count that can EXCEED any static seat, the direction the L-DO
	// variadic mark cannot express. The 1-seat layout was a live MISCOMPILE
	// (`size [(await {mode:'any'} [[7 8]])]` — interpreter 2, compiled a
	// stranded 7 and a 1-element list), so awaitVariadicResult now refuses the
	// compile pass wholesale and the interpreter owns these modes. Graduation
	// = a runtime-variadic region representation (an OpStackMark-style collect
	// with no static count); the refusal arm then records the region and the
	// rows move to lang/spec/module-time.tsv.
	`import "boru:time-util" TimeUtil.await {mode:'first'} [[1 2 3]]`:      {why: "NUR067: the winner's 3-value residual has no static seat", failsWith: "runtime-variadic (0-or-more values) with no static seat"},
	`import "boru:time-util" size [(TimeUtil.await {mode:'any'} [[7 8]])]`: {why: "NUR067: the miscompile shape — both values must reach the collecting paren", failsWith: "runtime-variadic (0-or-more values) with no static seat"},
	`import "boru:time-util" 99 TimeUtil.await {mode:'first'} [[]]`:        {why: "NUR067: an empty winner contributes nothing — the zero-count direction", failsWith: "runtime-variadic (0-or-more values) with no static seat"},

	// Net drivers — plan Phase 5: per-iteration mark/collect in the for: lowering.

	// GRADUATED 2026-07-14 (L-EACH, plan Phase 5): the three forward-drift
	// rows compile natively — errorReturnsFn narrows the catch result to
	// dynamic(join(pass-through, handler-residual)), so the String catch-all
	// overload is disjoint and check mode selects the interpreter's forward
	// collection (rows moved to lang/spec/bytecode-migrated.tsv; family
	// pinned in lang/go/bytecode_edge_findings_test.go with the genuinely
	// wide-join negative keeping the drift refusal).

	// do-unit registry replay — was a MISCOMPILE (variation sweep,
	// 2026-07-13); now a SOUND REFUSAL (drift graduated the same day): the
	// bake decision declines a body carrying a capitalised def
	// (bodyHasReplayHazard), so the interpreter owns the shape with full
	// parity. Full graduation = the Phase 6 JIT detached-unit cache compiles
	// these bodies as units (the check-time install becomes the only
	// install), at which point the rows compile and this entry deletes.
	// GRADUATED 2026-07-14: the do-unit registry-replay rows — do-def LEAK
	// fidelity (RunCarrierBodyKeepDefs) lets the closure re-analysis
	// shadow-rebind instead of tripping the parts conflict, so the typed-def
	// bodies compile as closure units with byte-identical results (rows
	// moved to lang/spec/bytecode-migrated.tsv; leak-semantics edges pinned
	// in lang/go/bytecode_replayhazard_test.go).

	// GRADUATED 2026-07-14: the L-JOIN recursive branch-join row — the
	// refusal was the disjunct-distribution recording per-alternative
	// (disjunctPartitionReturns combos under the armed recording); the fix
	// suspends the combo probes and records ONE CALL_USER with the original
	// args (carrier.go, gated by disjunctCombosTakeSig). Row moved to
	// lang/spec/bytecode-migrated.tsv; the family is pinned in
	// lang/go/bytecode_ljoin_test.go.

	// ───────────────────────────────────────────────────────────────────
	// frontier-hof-audit.tsv — the higher-order audit's §1 programs
	// (design/HIGHER-ORDER-FUNCTIONS.0.md §1, pinned 2026-08-21). Three
	// refusal families, all pre-existing and documented in the audit:
	//
	// (1) audit §5.8 / COMPILABLE-SUBSET.md "slow, not wrong": a curried
	//     combinator's body result is an inner fn literal closing over the
	//     enclosing parameters — unknown provenance, so the mint refuses
	//     and the interpreter owns the program. The CPS rows are the same
	//     family one step in: the continuation call `(k m)` inside a
	//     fn-local fn is a fn CALL operand of unknown provenance.
	`def chainif fn [[a:Function b:Function s:Integer][Any][def r1 (a s) if (r1.ok) [def r2 (b (r1.rest)) (r2.val)] [0]]] chainif ([z:Integer] => [{ok:true rest:8}]) ([z:Integer] => [{ok:true val:50}]) 4`: {why: "NUR087's branch-local def-split: the check pass is clean since the fix, but the branch arm's dispatch through a Function param takes the checker's best-fit recovery, and a recovered dispatch refuses compilation; graduation = a modelled branch-arm param dispatch", failsWith: "unmatched dispatch recovered at dot"},
	`def dbl x:Integer => [mul 2 x]  for-each dbl/v [1 2 3]`: {why: "for-each's Function form meets the Stage 3 function-valued-operand gate before the callback is even reached", failsWith: "function-valued operand at for-each (Stage 3)"},
	// Rewritten 2026-08-24 for the BROAD park (NUR073): the audit's §1
	// programs are respelled with explicit apply, so these keys are the
	// migrated TSV rows verbatim (literal keys — the shared hof* prefixes no
	// longer factor cleanly across the staged spellings).
	`def czero f:Function => [x:Any => [x/v]] end def csucc n:Function => [f:Function => [x:Any => [(x (n f/v apply) apply) f/v apply]]] end def toint n:Function => [0 ((k:Integer => [add k 1]) n/v apply) apply] end def c1 (csucc czero/v) end def c2 (csucc c1/v) end def c3 (csucc c2/v) end (toint c3/v)`:                                                                                                                                           {why: "RE-DIAGNOSED 2026-09-08 (the thirty-second increment): csucc's inner lambda reads `n` BARE beneath the paren-bounded apply (`(x (n f/v apply) apply)`), a word dispatch of the numeral over `f/v` the window would lower as data — the NUR123 accounting refuses it (it compiled to `f` applied to `n` while the accounting skipped units ending in a tail apply); the `n/v` spelling compiles. Graduation = a bare fn-typed read with a forward argument modelled as the dispatch it is", failsWith: "body result of unknown provenance"},
	`def czero f:Function => [x:Any => [x/v]] end def csucc n:Function => [f:Function => [x:Any => [(x (n f/v apply) apply) f/v apply]]] end def cplus m:Function => [n:Function => [f:Function => [x:Any => [(x (n f/v apply) apply) (m f/v apply) apply]]]] end def toint n:Function => [0 ((k:Integer => [add k 1]) n/v apply) apply] end def c1 (csucc czero/v) end def c2 (csucc c1/v) end def c3 (csucc c2/v) end (toint (c3/v (cplus c2/v) apply))`: {why: "audit §5.8: Church addition — csucc's bare `n` read beneath the paren-bounded apply (see the csucc row, the thirty-second increment)", failsWith: "body result of unknown provenance"},
	`def czero f:Function => [x:Any => [x/v]] end def csucc n:Function => [f:Function => [x:Any => [(x (n f/v apply) apply) f/v apply]]] end def cmult m:Function => [n:Function => [f:Function => [(f/v n/v apply) m/v apply]]] end def toint n:Function => [0 ((k:Integer => [add k 1]) n/v apply) apply] end def c1 (csucc czero/v) end def c2 (csucc c1/v) end def c3 (csucc c2/v) end (toint (c3/v (cmult c2/v) apply))`:                              {why: "audit §5.8: Church multiplication — csucc's bare `n` read beneath the paren-bounded apply (see the csucc row, the thirty-second increment)", failsWith: "body result of unknown provenance"},

	// (2) the §4.3 capture family: kk's inner lambda captures x, and the
	//     compiled capture is unreachable at the ((kk 7) 99) call site —
	//     the same refusal the audit's §4.3 kkA/B/C spellings draw.

	// (3) the def-bound computed-fn family, post the Stage 1 check-model
	//     fix (2026-08-21): a PLAIN read of a name def-bound to a computed
	//     fn now resolves through the per-pass fn-carrier side table on the
	//     compile lane (stepWord's consult — the false undefined_word is
	//     gone), so these rows refuse one or two stages LATER, each at a
	//     sound emit-land gate. The §5.6 freeze-idiom row graduated
	//     outright (deleted here — TestFrontierSpecCompiled now requires it
	//     to compile with parity). A `/v` read of such a name deliberately
	//     KEEPS the undefined_word diagnostic (stepWordVal declines the
	//     table): substituting there green-lights lowerings that drop the
	//     operand (the pmany/pseq shape below refused at the unmatched-
	//     dispatch recovery for the same reason — the trap declines a
	//     window naming a table-bound word, engine.go's
	//     TryRecordUnmatchedDispatchTrap).
	`def mk fn a:Integer Function [(fn b:Integer Integer [add a b])] end def h (mk 1) end 2 h/v apply`: {why: "the `/v` read of def-bound computed `h` keeps its undefined_word diagnostic (the deliberate Stage 1 /v hold; audit §5.4's workaround row — its ((mk 1) 2) sibling compiles natively as the unledgered control)", failsWith: "check diagnostics"},
	hofPitem + `def manyloop fn [[a:Function s:String acc:List][Map][ def r (a s) if (r.ok) [ (manyloop a/v (r.rest) (push (r.val) acc)) ] [ {ok:true val:acc rest:s} ] ]] end def pmany fn a:Function Function [ ( fn s:String Map [ def z [] (manyloop a/v s z) ] ) ] end def isdigit c:String => [ and (gte "0" c) (lte "9" c) ] end def digit (psat isdigit/v) end def digits (pmany digit/v) end (digits '123ab')`: {why: "RE-DIAGNOSED 2026-08-27 (NUR101): still refused, still the same interpreted answer, but the refusal MOVED EARLIER. psat's inner `if` arm nets a Function-typed carrier LEADING further values, and resolveArm now declines that shape (residualLeadReStepped) instead of merging it as placed data — the arm body closes through a frame rewind, so the interpreter re-steps the lead into a call and the merge would have compiled the placed pair. The pmany trap decline below is still there; it is simply no longer the FIRST refusal. Original diagnosis, still accurate for that later gate: `digit/v` at pmany's Function slot is check-invisible (digit is table-bound), so the dispatch no-match declines the trap and refuses", failsWith: "fn psat: body result of unknown provenance"},
	hofPalt + `(ab 'bzz')`: {why: "with the `ab` read resolved (Stage 1), palt's own unit refuses: its returned closure captures the alternation's parsers", failsWith: "body result of unknown provenance"},
	hofPalt + `(ab 'zzz')`: {why: "with the `ab` read resolved (Stage 1), palt's own unit refuses: its returned closure captures the alternation's parsers", failsWith: "body result of unknown provenance"},
	hofPitem + `def pseq fn [[a:Function b:Function][Function][ ( fn s:String Map [ def r1 (a s) if (r1.ok) [ def r2 (b (r1.rest)) if (r2.ok) [ {ok:true val:[(r1.val) (r2.val)] rest:(r2.rest)} ] [ {ok:false rest:s val:None} ] ] [ {ok:false rest:s val:None} ] ] ) ]] end def isdigit c:String => [ and (gte "0" c) (lte "9" c) ] end def digit (psat isdigit/v) end def two (pseq digit/v digit/v) end (two '42x')`: {why: "RE-DIAGNOSED 2026-08-27 (NUR101): the same earlier-refusal move as the pmany row — psat's arm nets a leading fn carrier that resolveArm now declines rather than merge as placed data. `digit/v` at pseq's Function slots is still check-invisible (digit is table-bound) and still declines the trap; it is no longer the first gate reached", failsWith: "fn psat: body result of unknown provenance"},

	// The §9 Stage-2 refusal rows: the lead-apply admission's witnesses
	// compile (unledgered), while these two spellings stay sound refusals.
	// §9d — the GRADUAL inner parameter (`x:Any` beside the captured
	// `g`) GRADUATED 2026-09-07 (the twenty-sixth increment): a lambda
	// VALUE unit takes the fn path's residual replay, whose applicable
	// count is names-aware, so the gradual `x` no longer competes with `g`
	// for the apply. Both spellings (the arrow-inner factory and its verbose
	// twin) moved to lang/spec/bytecode-migrated.tsv with the §1.6 compose
	// row and the §9 mk0 0-arg row (the fnval unit now models the raise).
	// §9e — the curried CHAIN through def bindings. `def f2 (f1 2)` binds
	// f2 to the very carrier f1 denotes (the analysis returns the callee
	// unchanged), which compiled put both names on one slot and leaked the
	// unconsumed argument into the residual — `2 fn (Integer) 3` for the
	// interpreter's `6`. A regression the Stage 1 read substitution
	// introduced (before it, the read raised undefined_word and the
	// program refused); the def site now detects the dropped apply.
	// §9f — code BODIES over def-bound computed fns. A code body is re-run
	// by its native through the INTERPRETER, and neither of this branch's
	// admissions survives that: the read substitution turns a body TOKEN
	// into a value, and a compiled ClosurePayload is invokable only through
	// the VM's re-entrant runner. All three were regressions found by a
	// differential sweep and are now sound refusals.
	`def mk fn [[a:Integer][Function][( fn [[b:Integer][Integer][add a b]] )]] end def f (mk 1) end each [1 2 3] [(f 1)]`: {why: "audit §5.8/§9f: an each body reading a def-bound computed fn — compiles and agrees since the thirty-sixth increment (the def claims the closure's shape), but the each body islands", failsWith: "islanded"},
	`def mk fn [[a:Integer][Function][( fn [[b:Integer][Integer][add a b]] )]] end def f (mk 1) end do [(f 2)]`:           {why: "audit §5.8/§9f: a do body reading a def-bound computed fn — the substitution declines in a nested body, restoring the pre-Stage-1 refusal", failsWith: "check diagnostics"},
	`def mkg g:Function => [v:Integer => [(g v)]] end def h (mkg (z:Integer => [add 7 z])) end do [(h 1)]`:                {why: "audit §5.8/§9f: a do body reading a def-bound COMPILED CLOSURE — an interpreter re-run cannot apply one", failsWith: "code body reads a def-bound compiled closure"},
	// §9g — a computed closure at a WORD's argument slot. Found by a
	// 690-program generated differential sweep (factory spelling x binding
	// shape x consumption context); 24 diverged, in exactly two contexts.
	// The interpreter APPLIES a paren-bounded call of a def-bound compiled
	// closure; the compiled model could leave the paren uncollapsed, so an
	// Any-typed slot swallowed the FUNCTION and stranded the argument.
	// Unmasked by 3d914ad — before the Stage 2 admission these refused.
	`def mk fn [[g:Function][Function][( fn [[v:Integer][Integer][(g v)]] )]] end def h (mk (z:Integer => [add 7 z])) end filter [1 2] [gt 0 (h 5)]`: {why: "audit §5.8/§9g: the same shape inside a filter body — compiles and agrees since the thirty-sixth increment (the read model over a def-bound closure), but the filter body islands", failsWith: "islanded"},
	// §9h — the two binding stores (both P1 findings of the #397 review). A
	// computed fn is not installed in Defs, so it lives only in the carrier
	// table and the stores can disagree. Shadowing a live binding leaves the
	// name with two meanings: compiled bound only the shadowed value, so this
	// answered `1 2` where the interpreter answers 3. Refuses now.
	`def mk fn [[a:Integer][Function][( fn [[b:Integer][Integer][add a b]] )]] end def f 1 end def f (mk 1) end undef f (f 2)`: {why: "audit §5.8/§9h: a computed fn shadowing a live binding — Defs and the carrier table disagree about the name", failsWith: "computed fn shadows a live binding"},

	// ───────────────────────────────────────────────────────────────────
	// frontier-fn-util.tsv — the boru:fn-util rows (audit §6.4 shipped
	// 2026-08-21). The eight BEHAVIOUR rows (compose, pipe, const, flip,
	// partial, on ×2, memoize) GRADUATED 2026-09-08 (the thirty-fifth
	// increment — the def-bound computed-fn model): the producing word's
	// check-mode ReturnsFn claims the wrapper's arity (CheckState.FnShapes),
	// the check pass models the def-bound wrapper's WORD DISPATCH at the read
	// over its arity of fixed tokens inside the statement (check's
	// tryShapedFnReadArrival → OpCallDynMethod), and the VM applies the
	// self-contained Go-impl wrapper on its OWN signatures. (The refusal's
	// old note — "a lowered apply here returned 99 for 7" — was the VM
	// resolving the wrapper's LABEL `const` through the live registry to the
	// singleton-type maker; the island it blamed applies the value exactly
	// as the interpreter does.) They live in lang/spec/module-fn.tsv now.
	//
	// The curried-chain row graduated with the thirty-sixth increment: the
	// claim carries the RESULT's shape (each curry level returns a unary
	// level), so the read's result is a shaped Function carrier and the
	// chain compiles level by level. Only the two strict-check rows remain.
	`import "boru:fn-util"  FnUtil.flip 5`:  {why: "strict-lane check: the FnUtil result is def-bound to a computed fn and unresolved (the frontier-hof-audit def-bound family; graduation = the def-bound computed-fn model)", failsWith: "check diagnostics"},
	`import "boru:fn-util"  FnUtil.curry 5`: {why: "strict-lane check: the FnUtil result is def-bound to a computed fn and unresolved (the frontier-hof-audit def-bound family; graduation = the def-bound computed-fn model)", failsWith: "check diagnostics"},

	// ───────────────────────────────────────────────────────────────────
	// frontier-while.tsv — the `while` word (closed audit §5.9's gap,
	// 2026-08-21). Four rows GRADUATED with the thirty-seventh increment
	// (2026-09-09): a while records on the counted loop's own frame
	// (RecordWhile — an unbounded FOR_SETUP/FOR_NEXT, the condition
	// fragment lowered at the head of every iteration, a falsy value
	// exiting through FLOW_BREAK); they live in lang/spec/control.tsv §7.
	// The two rows that remain refuse soundly, each under a gate that is
	// not the loop's.
	`def c (flex {n:0}) end while [(c get 'n') lt 3] [ set 'n' ((c get 'n') add 1) c end if ((c get 'n') eq 2) [continue] end (c get 'n') ]`: {why: "RE-DIAGNOSED 2026-09-09 (the thirty-seventh increment): the while itself lowers; the body's `if ((c get 'n') eq 2) [continue]` is a computed-condition no-else if whose one arm diverges, which the branch recorder refuses under `for` too (`if (n eq 2) [continue]` over a plain read compiles). Graduation = the diverging arm of a computed-condition no-else if", failsWith: "computed-branch non-eager arm diverges"},
	`while [] [1]`: {why: "RE-DIAGNOSED 2026-09-09 (the thirty-seventh increment): the lowering admits a condition netting exactly one value; an empty region nets none, so the recorder refuses where the interpreter raises its runtime_error. Graduation = a terminal trap for the statically-empty condition (RecordTrap, top level only)", failsWith: "condition nets 0 values, not one"},
}

type frontierEntryLS struct {
	why       string
	failsWith string
}

// TestFrontierSpecCompiled — the compile frontier: an unledgered row must
// compile NATIVELY (no island) and run with byte-identical parity; a
// ledgered row must refuse with the pinned reason (stale → graduate; drift →
// re-diagnose).
func TestFrontierSpecCompiled(t *testing.T) {
	for _, row := range loadFrontierRows(t) {
		err := frontierRowCompiles(row.input)
		key := row.input
		entry, ledgered := frontierCompileLedger[key]
		loc := fmt.Sprintf("%s:%d", row.file, row.line)
		switch {
		case !ledgered && err != nil:
			t.Errorf("%s: frontier row must COMPILE (not ledgered): %v\n  input: %s", loc, err, row.input)
		case ledgered && err == nil:
			t.Errorf("%s: stale compile-ledger entry — the row now compiles; graduate it (delete the entry; usually move the row into the main lang/spec corpus).\n  was red because: %s", loc, entry.why)
		case ledgered && entry.failsWith == "":
			t.Errorf("%s: unpinned compile-ledger row — record the failure mode. Observed: %v", loc, err)
		case ledgered && !strings.Contains(err.Error(), entry.failsWith):
			t.Errorf("%s: compile failure MODE drifted:\n  got:    %v\n  pinned: %q\nre-diagnose before editing the ledger", loc, err, entry.failsWith)
		}
	}
	for key := range frontierCompileLedger {
		found := false
		for _, row := range loadFrontierRows(t) {
			if row.input == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("orphan compile-ledger entry (no such frontier row): %.80s…", key)
		}
	}
}

// frontierRowCompiles asserts the TARGET compile behavior for one row:
// a native (island-free) Program plus byte-identical value/error parity.
func frontierRowCompiles(input string) error {
	a, err := lang.New()
	if err != nil {
		return err
	}
	a.SetClock(specClock)
	prog, reason, _, cerr := a.CompileCheck(input)
	if cerr != nil {
		return fmt.Errorf("check error: %v", cerr)
	}
	if prog == nil {
		return fmt.Errorf("refused: %s", reason)
	}
	if strings.Contains(prog.Disassemble(), "FALLBACK") {
		return fmt.Errorf("islanded: program embeds an OpFallback span")
	}
	b, err := lang.New()
	if err != nil {
		return err
	}
	b.SetClock(specClock)
	// A VM island the disassembly cannot show — an op re-entering the
	// interpreter through the VM's own island seams (`vm:island`,
	// `vm:island-resolved`: the apply word's re-step of an un-stamped fn
	// value, a dynamic window) — is the interp-entry census's debt, and
	// graduating such a row would move that debt into the main corpus; it
	// stays ledgered as "islanded" like an OpFallback span does. Only the
	// VM's seams: a native handler's own interpretation (a test runner
	// stepping its subject) is the handler's, as it is for the census.
	var seams []string
	disarm := b.ArmInterpEntryHook(func(ev lang.InterpEntry) {
		if ev.CheckMode || ev.Attribution != "" || !strings.HasPrefix(ev.Seam, "vm:island") {
			return
		}
		seams = append(seams, ev.Seam)
	})
	gotC, compiled, errC := b.RunCompiled(input)
	disarm()
	if !compiled {
		return fmt.Errorf("did not run compiled (err=%v)", errC)
	}
	if len(seams) > 0 {
		return fmt.Errorf("islanded: program re-enters the interpreter (%s)", seams[0])
	}
	c, err := lang.New()
	if err != nil {
		return err
	}
	c.SetClock(specClock)
	gotI, errI := c.RunInterp(input)
	if fmt.Sprint(errC) != fmt.Sprint(errI) {
		return fmt.Errorf("error parity: compiled %v vs interp %v", errC, errI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		return fmt.Errorf("value parity: compiled %v vs interp %v", gotC, gotI)
	}
	return nil
}

// refusalRowLedger pins the knownRefusals rows' TARGET failure modes: each
// must eventually compile via the sound runtime re-dispatch mechanism (plan
// Phase 3, OpDispatchRematch) and raise the interpreter-identical error.
// DERIVED from knownRefusals — the single source of truth for the row
// sources — so graduation is auto-coupled: deleting a knownRefusals entry
// drops its ledger row here, flipping this test's assertion for that row to
// the target (compile + byte-identical parity). The failure mode is the
// LEADING CLAUSE of the knownRefusals reason text ("branch leaves extra
// values (…)" pins "branch leaves extra values"): the remaining row's
// dispatch half already records an offset-form rematch, so its refusal
// signature is the branch-residual seat, not the dispatch recovery — a row
// developing a different failure mode trips the drift arm.
var refusalRowLedger = func() map[string]frontierEntryLS {
	m := make(map[string]frontierEntryLS, len(knownRefusals))
	for input, why := range knownRefusals {
		mode := why
		if i := strings.Index(mode, " ("); i > 0 {
			mode = mode[:i]
		}
		m[input] = frontierEntryLS{
			why:       "plan Phase 3/5 (OpDispatchRematch + variadic-region merge): " + why,
			failsWith: mode,
		}
	}
	return m
}()

// TestFrontierRefusalRowsCompile asserts the TARGET for every knownRefusals
// row: CompileCheck yields a Program and RunCompiled matches the
// interpreter's error byte-for-byte. All 9 are expected-red until Phase 3
// lands, ratcheting down row-by-row in lockstep with knownRefusals.
func TestFrontierRefusalRowsCompile(t *testing.T) {
	for input := range knownRefusals {
		err := frontierRowCompiles(input)
		entry, ledgered := refusalRowLedger[input]
		switch {
		case !ledgered && err != nil:
			t.Errorf("knownRefusals row must COMPILE (not ledgered — did Phase 3 graduate it?): %v\n  input: %.100s", err, input)
		case ledgered && err == nil:
			t.Errorf("stale refusal-ledger entry — the row now compiles; graduate BOTH ledgers (delete here AND in knownRefusals).\n  input: %.100s\n  was red because: %s", input, entry.why)
		case ledgered && entry.failsWith == "":
			t.Errorf("unpinned refusal-ledger row — record the failure mode. Observed: %v\n  input: %.100s", err, input)
		case ledgered && !strings.Contains(err.Error(), entry.failsWith):
			t.Errorf("refusal row failure MODE drifted:\n  got:    %v\n  pinned: %q\n  input: %.100s", err, entry.failsWith, input)
		}
	}
	for input := range refusalRowLedger {
		if _, ok := knownRefusals[input]; !ok {
			t.Errorf("orphan refusal-ledger entry (row left knownRefusals): %.80s…", input)
		}
	}
}
