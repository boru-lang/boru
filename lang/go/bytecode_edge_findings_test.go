package lang

import (
	"fmt"
	"strings"
	"testing"
)

// Landing tests for design/EDGE-SPEC-FINDINGS.0.md — four compile≠interpret
// divergences the edge-spec expansion surfaced. Each was a shape the compiler
// lowered to a WRONG value; the fix makes the compiler DECLINE (§5), after
// which the program is silently re-run on the interpreter — scaffolding
// absorbing an open compile defect, not a path the design owns. Every
// finding is pinned
// three ways: the reproducer DECLINES with its reason, the reproducer's compiled
// run falls back to interpreter PARITY, and a sibling that must keep compiling
// natively still does (the negative that proves the compile failure is not blanket).

// mustFailToCompileWithParity asserts src fails to compile (reason contains want)
// and that the interpreted run still produces want's answer.
func mustFailToCompileWithParity(t *testing.T, src, want string) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("%q: CompileCheck error %v", src, cerr)
	}
	if prog != nil {
		t.Fatalf("%q: expected a compile failure, but it compiled", src)
	}
	if !strings.Contains(reason, want) {
		t.Errorf("%q: compile failure reason = %q, want it to contain %q", src, reason, want)
	}
	// The compile failure is reported plainly and BOOKED, and the program
	// stays fully serviceable on the reference engine. Booking must not
	// short-circuit that second half: every source this helper takes fails
	// to compile, so an early return here would make the interpreter
	// assertion below dead code and let a real interpreter regression pass
	// as long as the defect count held.
	b, _ := New()
	_, compiled, errC := b.RunCompiled(src)
	requireCompileDefect(t, src, nil, errC)
	if compiled {
		t.Errorf("%q: RunCompiled reported a compiled run; a program that does not compile must not compile", src)
	}
	c, _ := New()
	if _, errI := c.RunInterp(src); errI != nil && codeOf(errI) == "compile_failed" {
		t.Errorf("%q: RunInterp must never report compile_failed", src)
	}
}

// mustCompileWithParity asserts src compiles natively (no whole-program
// fallback island in the reason) and RunCompiled matches the interpreter.
// interpOnlyWithCompileFailure asserts the interpreter's answer and TOLERATES a
// sound compile failure: since the BROAD park (NUR073 clause 3) a fetched fn
// reaches `apply` as an untyped carrier, and the record declines ("apply over
// a dynamic lead") rather than lower an unprovable overload — the default
// lane does not compile. Graduating the shape re-tightens the
// pin to mustCompileWithParity.
func interpOnlyWithCompileFailure(t *testing.T, src, want string) {
	t.Helper()
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gotI, errI := c.RunInterp(src)
	if errI != nil {
		t.Fatalf("%q: interp: %v", src, errI)
	}
	if want != "" && fmt.Sprint(gotI) != want {
		t.Errorf("%q: interp got %v, want %s", src, gotI, want)
	}
	a, _ := New()
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("%q: CompileCheck: %v", src, cerr)
	}
	if prog == nil {
		t.Logf("%q: compile declined (%q)", src, reason)
		return
	}
	b, _ := New()
	gotC, compiled, errC := b.RunCompiled(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if !compiled || errC != nil {
		t.Fatalf("%q: compiled run: compiled=%v err=%v", src, compiled, errC)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("%q: parity compiled=%v interp=%v", src, gotC, gotI)
	}
}

func mustCompileWithParity(t *testing.T, src, want string) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("%q: expected a native compile, declined: reason=%q err=%v", src, reason, cerr)
	}
	b, _ := New()
	gotC, compiled, errC := b.RunCompiled(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if !compiled || errC != nil {
		t.Fatalf("%q: compiled run: compiled=%v err=%v", src, compiled, errC)
	}
	c, _ := New()
	gotI, _ := c.RunInterp(src)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("%q: parity compiled=%v interp=%v", src, gotC, gotI)
	}
	if want != "" && fmt.Sprint(gotC) != want {
		t.Errorf("%q: got %v, want %s", src, gotC, want)
	}
}

// §1 — forward collection reaching across an error-handler residual. `add`
// used to match all-stack `[dynamic-error-out, leading-residual]` under the
// dynamic(Any) island output (its String catch-all), stranding the forward
// token the interpreter collects (`5 do … error [drop 9] add 1` → `5 10`
// interp, `14 1` compiled). GRADUATED (L-EACH, plan Phase 5): errorReturnsFn
// narrows the catch result to dynamic(join(pass-through, handler)) — here
// dynamic(Integer) — so the String overload is disjoint, check mode selects
// the interpreter's forward collection, and the rows compile natively. A
// GENUINELY wide join (an Integer|String boundary) keeps the drift compile failure —
// pinned below. Words whose forward collection was never blocked (`mul`,
// `sub`, a String forward token) keep compiling as before.
func TestEdgeFindingForwardAcrossErrorResidual(t *testing.T) {
	mustCompileWithParity(t, `5 do [raise aa "x"] error [drop 9] add 1`, "[5 10]")
	mustCompileWithParity(t, `5 do [7] error [drop 9] add 1`, "[5 8]")
	mustCompileWithParity(t, `1 2 3 do [7] error [drop 9] add 1`, "[1 2 3 8]")

	// GRADUATED (COMPILE FAILURE-CLOSURE §1, 2026-07-16 — the drift window): the
	// genuinely dynamic boundary now compiles as a TERMINAL OpCallDynamicMixed
	// island. tryRecordDriftWindow records the whole window — [leading
	// residual, catch-result, the word as an inert const, the forward
	// literal] — as one event; the VM re-steps it verbatim, so the island's
	// own dispatch performs the interpreter's forward collection over the
	// LIVE value: the Integer pass-through forward-collects (`7 add 1` →
	// [5 8]) while the String handler path binds all-stack — byte-identical
	// on both paths, residual count included (the event is variadic).
	mustCompileWithParity(t, `5 do [7] error ["x"] add 1`, "[5 8]")
	mustCompileWithParity(t, `5 do [raise aa 'm'] error ['x'] add 1`, "[5x 1]")
	mustCompileWithParity(t, `5 do [raise aa 'm'] error ['x'] add 'y'`, "[5 xy]")

	// The window's decline fences keep the compile failure: a NON-TERMINAL
	// drift site (a downstream consumer would need a result count the island
	// cannot promise) and BYSTANDER data below the window (the in-order
	// reconciliation cannot interleave the window's const re-pushes with
	// values the dispatch never touched).
	mustFailToCompileWithParity(t,
		`5 do [7] error ["x"] add 1 drop`,
		"forward operand accounting across a dynamic/island residual")
	mustFailToCompileWithParity(t,
		`1 2 3 do [7] error ["x"] add 1`,
		"forward operand accounting across a dynamic/island residual")

	// Negatives — the drift guard must NOT over-decline: `mul`/`sub` forward-
	// collect their token (fwdCount>0), a String forward token routes to a
	// different overload, and the no-leading-residual and concrete-`do` forms
	// have no bystander to reach past.
	mustCompileWithParity(t, `5 do [7] error [drop 9] mul 2`, "[5 14]")
	mustCompileWithParity(t, `5 do [7] error [drop 9] sub 1`, "[5 6]")
	mustCompileWithParity(t, `5 do [7] error [drop 9] add "z"`, "[5 7z]")
	mustCompileWithParity(t, `do [7] error [drop 9] add 1`, "[8]")
	mustCompileWithParity(t, `5 do [9] add 1`, "[5 10]")

	// A word with NoEvalArgs (code-body) positions — `if` here — forward-
	// collects its BODY tokens, never a trailing SCALAR after them, so the
	// drift model (a bare-scalar re-collection like `add 1`) does not apply.
	// `if ok [t] [e] 0` with a dynamic `ok` matched all-stack (an analysis-order
	// artifact) and a trailing `0` used to false-positive here; the recorded
	// branch binds the correct operands and the `0` is a separate residual, so
	// it compiles with parity. This is the decision prop_test summary-each shape
	// (`each [ … if ok [print] [print] 0 ]` over dynamic-typed results).
	mustCompileWithParity(t,
		`def xs [{ok:true} {ok:false}] def _ (xs each [ var [[r] def ok (r "ok" get) def res (if ok [1] [2]) def _2 res 0 ] ]) 9`,
		"[9]")
	mustCompileWithParity(t,
		`def m {ok:true} def ok (m get "ok") if ok [ 1 print ] [ 2 print ] 7`,
		"[7]")
}

// §2 — an applied member-fn boundary mid-expression. A parked fn read from a
// container (`m.double`) auto-applies the moment a value lands on it; the
// compiler previously let a downstream word (`eq`) steal that value, so the
// shape declined ("member fn value auto-applies mid-expression"). GRADUATED
// 2026-07-16 (COMPILE FAILURE-CLOSURE.0 §3, the arrival-apply model): the member-fn
// read tag now carries the pinpointed member VALUE, and when the member's
// single plain signature's arity of inert tokens follows the carrier,
// tryMemberFnArrivalDispatch models the interpreter's auto-dispatch at the
// ARRIVAL — a guarded mid-stream OpCallDynMethod whose runtime value (the
// real container read's product) drives the apply — so `m.double 21` runs
// BEFORE `eq`, exactly the interpreter's order. The paren-bounded variant
// graduates with it (the model fires inside the paren).
func TestEdgeFindingMemberFnApplyMidExpression(t *testing.T) {
	mustCompileWithParity(t,
		`def d fn [[n:Integer] [Integer] [n mul 2]] def m {double: d/v} m.double 21 eq 42`, "[true]")
	mustCompileWithParity(t,
		`def d fn [[n:Integer] [Integer] [n mul 2]] def m {double: d/v} (m.double 21) eq 42`, "[true]")
	// The String-typed twin: the arrival window binds the member's own sig.
	mustCompileWithParity(t,
		`def s fn [[x:String] [String] [x]] def m {id: s/v} m.id 'v' eq 'v'`, "[true]")
	// The arity-2 window (the adversarial review's requested pin): the model
	// claims the member sig's FULL arity of inert tokens.
	mustCompileWithParity(t,
		`def d fn [[a:Integer b:Integer] [Integer] [a add b]] def m {add2: d/v} m.add2 1 2 eq 3`, "[true]")

	// Negatives: the bare statement-tail apply keeps compiling; unapplied
	// member reads stay data; a non-fn member read never auto-applies.
	mustCompileWithParity(t,
		`def d fn [[n:Integer] [Integer] [n mul 2]] def m {double: d/v} m.double 21`, "[42]")
	mustCompileWithParity(t, `def m {x: 5} m.x eq 5`, "[true]")
	mustCompileWithParity(t, `def m {a: [1 2 3]} m.a get 0 eq 1`, "[true]")

	// The model's decline fence: a MULTI-overload member cannot claim one
	// arity window (runtime first-match not modelled) — the shape keeps a
	// a compile failure; the interpreted answer is asserted alongside via the hatch-free
	// Stage-J contract (RunCompiled declines; RunInterp owns it).
	multi := `def d fn [[n:Integer] [Integer] [n mul 2] [x:String] [Integer] [9]] def m {double: d/v} m.double 21 eq 42`
	a, _ := New()
	if prog, _, _, _ := a.CompileCheck(multi); prog != nil {
		t.Errorf("multi-overload member arrival must keep declining (sound fence)")
	}
	b, _ := New()
	if got, errI := b.RunInterp(multi); errI != nil || fmt.Sprint(got) != "[true]" {
		t.Errorf("multi-overload member interp = %v (err=%v), want [true]", got, errI)
	}
}

// §3 — a paren-arrived value run as an else body. `(range 2 4)` reaches the
// compiler as a non-concrete list carrier: the branch value path used to push
// the LIST while the interpreter's spliceArg EXECUTES it, so the shape declined
// ("computed branch arm is a spliced list body"). GRADUATED 2026-07-16
// (COMPILE FAILURE-CLOSURE.0 §4): computedArmDoBody synthesizes the equivalent
// `[do <arm>]` body — probes prove arm-splice ≡ do on every axis (multi-
// values, def leaking, break/continue via the FlowCtrl escape) — and the arm
// takes the ordinary body path, with the dyn-body machinery owning the
// computed `do`. Both arms, either position, taken or not.
func TestEdgeFindingComputedElseBody(t *testing.T) {
	mustCompileWithParity(t, `def n 5 if (n eq 0) [99] (range 2 4)`, "[2 3]")
	mustCompileWithParity(t, `def n 0 if (n eq 0) (range 2 4) [99]`, "[2 3]")
	mustCompileWithParity(t, `def n 5 if (n eq 0) [99] (range 2 3)`, "[2]")

	// The formerly-negative siblings keep compiling identically: the DEAD
	// spliceable arm, scalar/paren-scalar value arms, both-literal bodies.
	mustCompileWithParity(t, `def n 0 if (n eq 0) [99] (range 2 4)`, "[99]")
	mustCompileWithParity(t, `def n 5 if (n eq 0) (range 2 4) [99]`, "[99]")
	mustCompileWithParity(t, `def n 5 if (n eq 0) [99] 42`, "[42]")
	mustCompileWithParity(t, `def n 5 if (n eq 0) [99] (add 1 2)`, "[3]")
	mustCompileWithParity(t, `def n 0 if (n eq 0) [99] [88]`, "[99]")

	// The synthesized do-arm keeps splice semantics under FLOW and DEF axes
	// compiled: a break inside the computed arm escapes the enclosing loop,
	// and a def inside it leaks to the enclosing scope.
	mustCompileWithParity(t, `def body (quote [break]) for 3 [ if (i eq 1) body [] i ]`, "[0]")
	mustCompileWithParity(t, `def body (quote [def zz 7 zz]) def n 5 (if (n eq 0) [99] body) zz add 1`, "[7 8]")
}

// §5 (divergence fix) — a `do` body arriving as a QUOTED VALUE (via
// `def b (quote [break])`) runs its tokens AS CODE, so a break/continue inside
// escapes `do` to the enclosing loop exactly as the interpreter's shared tape
// does. Two bugs made the compiled run diverge: (a) bodyHasSentinel honoured
// the .Quoted flag and missed the break, so the const-folded body compiled into
// a closure whose break raised "flow signal with no enclosing loop" and `do`
// caught it as an error VALUE; (b) even routed to a FALLBACK `do`, the VM did
// not translate the escaped FlowCtrl on the fallback seam. Fixed:
// valueHasSentinel ignores .Quoted (routing the body off the closure path), and
// OpFallback now resolves escaped flow like the fn-value-call family. The break
// now unwinds the compiled loop → interpreter parity.
func TestEdgeFindingQuotedDoBodyFlowEscapesLoop(t *testing.T) {
	// The previously-miscompiling shapes now compile with parity: the break
	// terminates the enclosing `for`, so nothing is collected.
	mustCompileWithParity(t, `def b (quote [break]) for 5 [do b i]`, "[]")
	mustCompileWithParity(t, `def b (quote [continue]) for 3 [do b i]`, "[]")
	// A nested loop: the inner break terminates only the INNER `for`, so the
	// outer loop's `i` values survive.
	mustCompileWithParity(t, `def b (quote [break]) for 2 [ for 3 [do b] i ]`, "[0 1]")

	// Negatives — the fix must not over-decline a sentinel-free quoted do body.
	mustCompileWithParity(t, `def b (quote [7]) for 3 [do b i]`, "[7 0 7 1 7 2]")

	// No ENCLOSING loop: the escaped break has nowhere to unwind to, so the
	// fallback seam's resolveEscapedFlow returns flowSignal's no-loop error and
	// the compiled run raises the interpreter's canonical "break outside loop"
	// — parity on the RAISE, not the value (mustCompileWithParity can't express
	// a raising row). Covers the vm.go OpFallback error arm.
	{
		src := `def b (quote [break]) do b`
		a, _ := New()
		_, iErr := a.RunInterp(src)
		b, _ := New()
		_, _, cErr := b.RunCompiled(src)
		if noteCompileDefect(t, src, nil, cErr) {
			return
		}
		if iErr == nil || cErr == nil || codeOf(iErr) != codeOf(cErr) {
			t.Errorf("%q: raise parity — compiled=[%s]%v interp=[%s]%v",
				src, codeOf(cErr), cErr, codeOf(iErr), iErr)
		}
	}
}

// §4 — `args.N` inside a compiled fn body with UNNAMED params. Unnamed params
// now bind to frame locals exactly like named ones (CompiledFn.NUnnamed: RET
// discards the unconsumed frame-bottom copies the interpreter's body splice
// leaves), so `args.N` folds to PUSH_LOCAL N for EVERY frame shape and these
// previously-declined rows compile with parity.
func TestEdgeFindingArgsOverUnnamedParams(t *testing.T) {
	mustCompileWithParity(t,
		`def f fn [[Integer String] [String] [args.1]] f 1 "hi"`, "[hi]")
	mustCompileWithParity(t,
		`def f fn [[Integer String] [Integer] [args.0]] f 1 "hi"`, "[1]")
	mustCompileWithParity(t,
		`def f fn [[a:Integer Integer] [Integer] [args.0]] f 3 4`, "[3]")

	// Every param NAMED — `args.N` folds to PUSH_LOCAL N.
	mustCompileWithParity(t,
		`def f fn [[a:Integer b:String] [String] [args.1]] f 1 "hi"`, "[hi]")
	mustCompileWithParity(t,
		`def f fn [[a:Integer b:Integer] [Integer] [args.0 add args.1]] f 3 4`, "[7]")
	mustCompileWithParity(t,
		`def f fn [[n:Integer] [Integer] [if (n lte 0) [args.0] [f (n sub 1)]]] f 3`, "[0]")
}

// Conditional fn-shadow divergence — a user fn REDEFINED inside a branch/loop
// body clobbers the enclosing overload in place (installDef's overlap-removal
// drops the outer entry without growing the def depth, so the branch/loop
// rollback cannot restore it). Compiled resolution then statically bakes the
// conditional shadow while the interpreter keeps the outer fn when the branch
// is not taken (or the loop runs zero times), so `if false [def g …] g 1`
// returned the shadow's value compiled but the ORIGINAL interpreted. The fix
// fails to compile the redefinition (CondBodyDepth-gated), and the program
// is silently re-run on the interpreter — contained, not fixed, and the shape
// is still owed a lowering.
func TestEdgeFindingConditionalFnShadowFailsToCompile(t *testing.T) {
	fnA := `fn [[x:Any] [Integer] [x add 100]]`
	fnB := `fn [[x:Any] [Integer] [x add 1]]`
	want := "redefined inside a conditional body"

	// DECLINE: every conditionally-reached redefinition of an outer fn.
	mustFailToCompileWithParity(t, `def g `+fnA+` if false [def g `+fnB+`] g 1`, want) // branch not taken
	mustFailToCompileWithParity(t, `def c false def g `+fnA+` if c [def g `+fnB+`] g 1`, want)
	mustFailToCompileWithParity(t, `def g `+fnA+` if true [def g `+fnB+`] g 1`, want) // taken, still unsound-at-shape
	mustFailToCompileWithParity(t, `def g `+fnA+` for 2 [def g `+fnB+`] g 1`, want)   // loop body
	mustFailToCompileWithParity(t, `def g `+fnA+` ([1 2] each [def g `+fnB+`]) g 1`, want)

	// COMPILE (must NOT over-decline): the redefinition is UNCONDITIONAL.
	mustCompileWithParity(t, `def g `+fnA+` def g `+fnB+` g 1`, "[2]")      // top-level shadow
	mustCompileWithParity(t, `def g `+fnA+` do [def g `+fnB+`] g 1`, "[2]") // do leaks unconditionally
	// COMPILE: no outer overload to clobber — a NEW name defined in a branch.
	mustCompileWithParity(t, `if true [def h `+fnB+` h 1]`, "[2]")
	// COMPILE: the control fn with no shadow at all.
	mustCompileWithParity(t, `def g `+fnA+` g 1`, "[101]")
}

// The §3 arrival-apply model's decline fences, each with engine parity: a
// shape the model cannot claim keeps a SOUND outcome (a native compile of the
// unaffected form, or a compile failure whose compile failure/Stage-J contract
// holds). One table so every fence stays exercised (the cover-gate demands
// each decline arm).
func TestMemberFnArrivalDeclineFences(t *testing.T) {
	cases := []struct {
		name, src string
		compiles  bool
		want      string // interp result (fmt.Sprint)
	}{
		// A computed key cannot pinpoint the member: the tag rides bool-only
		// and the arrival model declines — but the fetched fn reaches `apply`
		// as a GRADUAL lead over one receiver, which the record lowers as
		// the apply EVENT (OpCallDynApplyOne) at the program level since the
		// dynamic-lead group (2026-09-22; it was "apply over a dynamic lead
		// (overload unprovable)" — the event was unit-only before).
		{"computed key", `def d fn [[n:Integer][Integer][n mul 2]] def m {double: d/v} def k (do [double/q]) 21 (m get k) apply eq 42`, true, "[true]"},
		// A LIST member pinpoints by concrete index — the arrival model fires.
		{"list member", `def d fn [[n:Integer][Integer][n mul 2]] def lst [d/v] 21 (lst get 0) apply eq 42`, true, "[true]"},
		// Anonymous lambda member: no name for the model — compile failure.
		{"anonymous member", `def m {double: ([n:Integer] => [n mul 2])} m.double 21 eq 42`, false, "[true]"},
		// 0-arg member: the arrival model claims the empty-window arity-0
		// landing (the break-2 closure, FN-VALUE-OPEN-WORK §4) — the
		// courtesy dispatch compiles as an arity-0 OpCallDynMethod.
		{"zero-arg member", `def z fn [[][Integer][7]] def m {z: z/v} m.z eq 7`, true, "[true]"},
		// Quoted-param member: the arrival model's plain-value-args
		// assumption fails, so the COMPILE declines — but the interpreter
		// now runs it right: the NUR038 arrival path converts the bare
		// word through the /q slot (`m.q foo` ≡ `q foo` → 9, then 9 eq 9).
		{"quoted param member", `def q fn [[k:Atom/q][Integer][9]] def m {q: q/v} m.q foo eq 9`, false, "[true]"},
		// Two-return member: the single-result claim fails — compile failure.
		{"two-return member", `def t fn [[n:Integer][Integer Integer][n n]] def m {t: t/v} m.t 3 eq 3`, false, "[3 true]"},
		// The member read ends the tape: no window — the fn stays data.
		{"read at tape end", `def d fn [[n:Integer][Integer][n mul 2]] def m {double: d/v} m.double`, true, "[fn d(Integer)]"},
		// A word right after the carrier: the window is not inert.
		{"non-inert window", `def d fn [[n:Integer][Integer][n mul 2]] def m {double: d/v} def x 21 m.double x eq 42`, false, "[true]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, cerr := a.CompileCheck(c.src)
			if cerr != nil {
				t.Fatalf("CompileCheck: %v", cerr)
			}
			if (prog != nil) != c.compiles {
				t.Errorf("compiles=%v want %v (reason=%q)", prog != nil, c.compiles, reason)
			}
			b, _ := New()
			gotC, ran, _ := b.RunCompiled(c.src)
			d2, _ := New()
			gotI, errI := d2.RunInterp(c.src)
			if errI != nil || fmt.Sprint(gotI) != c.want {
				t.Errorf("interp = %v (err=%v), want %s", gotI, errI, c.want)
			}
			if ran && fmt.Sprint(gotC) != fmt.Sprint(gotI) {
				t.Errorf("parity: compiled=%v interp=%v", gotC, gotI)
			}
		})
	}
}

// The CLASS-INSTANCE member-fn arrival family — the formerly PRE-EXISTING
// stranded-apply miscompile (`o.f 21 eq 42` compiled to the fn-as-data plus
// eq(21,42)=false where the interpreter applies the member → true; the make
// result is a CARRIER at the read site, so the concrete-container tag walk
// never saw the member). Fixed by the construction-time fnMemberFields note:
// `make` remembers every fn-valued field VALUE from its concrete map, the
// get-family tag site consults it through the carrier (instanceFnMember),
// and the landing routes through the same §3 arrival model / stranded-fn
// guard as a map-member read.
func TestInstanceMemberFnArrival(t *testing.T) {
	// The miscompile shape compiles with parity now (the arrival model).
	mustCompileWithParity(t,
		`def d fn [[n:Integer][Integer][n mul 2]] def C class {f: Function} def o (make C {f: d/v}) o.f 21 eq 42`, "[true]")
	// The statement-tail apply and the bare (unapplied) read keep compiling.
	mustCompileWithParity(t,
		`def d fn [[n:Integer][Integer][n mul 2]] def C class {f: Function} def o (make C {f: d/v}) o.f 21`, "[42]")
	// A shape the arrival model declines — a WORD right after the carrier —
	// must now DECLINE via the stranded-fn guard (sound), never miscompile.
	declined := `def d fn [[n:Integer][Integer][n mul 2]] def C class {f: Function} def o (make C {f: d/v}) def x 21 o.f x eq 42`
	a, _ := New()
	if prog, reason, _, _ := a.CompileCheck(declined); prog != nil {
		t.Errorf("the declined instance landing must decline (reason=%q):\n%s", reason, prog.Disassemble())
	}
	b, _ := New()
	if got, errI := b.RunInterp(declined); errI != nil || fmt.Sprint(got) != "[true]" {
		t.Errorf("declined-shape interp = %v (err=%v), want [true]", got, errI)
	}
}

// PR #275 review finding (P1) — valueHasSentinel missed break/continue nested
// inside INTERPOLATED literal expression parts (string `${...}`, XML attribute
// and child holes) and MAP values, all of which run as code when the container
// materialises in a do-body. The scanner returned false, the do-body compiled
// to a closure, and the escaped signal surfaced as "flow signal with no
// enclosing loop" error values instead of breaking the outer loop. The fix
// recurses into all three container families (mirroring walkBodyValue), so the
// closure compile declines and the fallback seam threads the signal — parity.
func TestEdgeFindingSentinelInInterpolatedParts(t *testing.T) {
	// The reported fixture: interp-string ${break} in a quoted do-body.
	mustCompileWithParity(t, "def b (quote [`${break}`]) for 5 [do b i]", "[]")
	mustCompileWithParity(t, "def b (quote [`${continue}`]) for 3 [do b i]", "[]")
	// XML interpolation: child hole, attribute hole, NESTED child template.
	mustCompileWithParity(t, `def b (quote [<p>${break}</p>]) for 5 [do b i]`, "[]")
	mustCompileWithParity(t, `def b (quote [<p a=${break}></p>]) for 5 [do b i]`, "[]")
	mustCompileWithParity(t, `def b (quote [<p><q>${break}</q></p>]) for 5 [do b i]`, "[]")
	// Map literal: values evaluate when the literal assembles.
	mustCompileWithParity(t, `def b (quote [{k: break}]) for 5 [do b i]`, "[]")
	// An APPLIED anonymous fn is raw tokens at scan time (`fn` + sig/body
	// lists), so the body-list recursion sees its break — and the interpreter
	// does propagate an applied callee's break to the enclosing loop.
	mustCompileWithParity(t, "def b (quote [`${1 (fn [[x:Integer] [Integer] [break 7]]) apply}`]) for 5 [do b i]", "[]")
	mustCompileWithParity(t, `def b (quote [1 (fn [[x:Integer] [Integer] [break 7]]) apply]) for 5 [do b i]`, "[]")

	// Negatives — sentinel-free interpolations/maps must KEEP compiling.
	mustCompileWithParity(t, "def b (quote [`v${1 add 1}`]) for 2 [do b i]", "[v2 0 v2 1]")
	mustCompileWithParity(t, `def b (quote [<p>${1 add 1}</p>]) for 2 [do b i]`, "[<p>2</p> 0 <p>2</p> 1]")
	mustCompileWithParity(t, `def b (quote [<p><q>${1 add 1}</q></p>]) for 2 [do b i]`, "[<p><q>2</q></p> 0 <p><q>2</q></p> 1]")
	mustCompileWithParity(t, `def b (quote [{k: 7}]) for 2 [do b i]`, "[{k:7} 0 {k:7} 1]")
	// A typed-map body element (ChildTypeInfo — Parent=TMap, non-OrderedMap
	// payload) rides the scanner's nil-AsMap guard and keeps compiling.
	mustCompileWithParity(t, `def b (quote [{:String}]) for 2 [do b i]`, "[{:String} 0 {:String} 1]")

	// TRANSITIVE sentinels (pre-existing on main): a NAMED fn whose body holds
	// a bare break, CALLED from the do-body. The syntactic scanner sees only
	// the word `f`, but the interpreter unwinds the callee's break to the
	// enclosing loop — while a compiled do-closure starts a fresh loop stack
	// (invokeClosureOn, unlike OpCallUser's loopBase frames) and surfaced
	// flow-signal error values. bodyHasSentinelDeep resolves body words to
	// user-fn bodies (recursively, cycle-guarded) at the tryRecordDynBody
	// gate, so the closure compile declines and the fallback threads the
	// signal — parity, for the quoted, inline-literal, and two-hop shapes.
	fnBreak := `def f fn [[x:Integer] [Integer] [break 7]] `
	mustCompileWithParity(t, fnBreak+`def b (quote [f 1]) for 5 [do b i]`, "[]")
	mustCompileWithParity(t, fnBreak+`for 5 [do [f 1] i]`, "[]")
	mustCompileWithParity(t,
		fnBreak+`def g fn [[x:Integer] [Integer] [f x]] def b (quote [g 1]) for 5 [do b i]`, "[]")
	// Recursive callee: the seen-set terminates the scan (and the shape
	// declines conservatively — parity rides the fallback).
	mustCompileWithParity(t,
		`def r fn [[n:Integer] [Integer] [if (n lte 0) [break 0] [r (n sub 1)]]] def b (quote [r 2]) for 5 [do b i]`, "[]")
	// The direct-call sibling keeps parity natively: OpCallUser frames share
	// the caller's loop stack, so the callee's escaped break lands in the loop.
	mustCompileWithParity(t, fnBreak+`for 5 [f 1]`, "[]")
	// Negative — a sentinel-free callee must NOT decline the do-body compile.
	mustCompileWithParity(t,
		`def g fn [[x:Integer] [Integer] [x add 1]] def b (quote [g 1]) for 2 [do b i]`, "[2 0 2 1]")
}

// PR #275 review finding (P2) — the CondBodyDepth raise (conditional fn-shadow
// compile failure) over-applied to list-form `if` CONDITIONS and `case` code-body
// scrutinees, which run unconditionally exactly once BEFORE the branch
// decision: a same-sig redefinition there is not path-dependent, and the
// equivalent paren-`do` condition already compiled with parity. The fix routes
// analyseCondFragment through RunCarrierCondBody (CondBodyDepth-exempt);
// branch arms and loop bodies keep the raise (TestEdgeFindingConditionalFnShadowFailsToCompile).
func TestEdgeFindingCondFragmentRedefCompiles(t *testing.T) {
	fnA := `fn [[x:Any] [Integer] [x add 100]]`
	fnB := `fn [[x:Any] [Integer] [x add 1]]`

	// The reported fixture: redefinition inside the list-form if condition.
	mustCompileWithParity(t,
		`def g `+fnA+` if [def g `+fnB+` true] [0] [9] g 1`, "[0 2]")
	// Its paren-`do` twin (the semantic reference) keeps compiling.
	mustCompileWithParity(t,
		`def g `+fnA+` if (do [def g `+fnB+` true]) [0] [9] g 1`, "[0 2]")
	// 2-arg if condition rides the same fragment path.
	mustCompileWithParity(t,
		`def g `+fnA+` if [def g `+fnB+` true] [0] g 1`, "[0 2]")
	// `case` code-body scrutinee: runs once before dispatch — also exempt.
	mustCompileWithParity(t,
		`def g `+fnA+` case [def g `+fnB+` 5] [5 88 99] g 1`, "[88 2]")

	// A redefinition in an ARM under a condition the model cannot decide
	// (a code-body condition) is PLACED since the seventieth increment:
	// the arm's install at its site through the interpreter's own
	// installer, the dispatch routed on the live lead — parity on the
	// taken path here, and on the not-taken path in
	// TestConditionalFnDefIsSpeculative.
	mustCompileWithParity(t,
		`def p 5 def g `+fnA+` if [p gt 3] [def g `+fnB+` 0] [9] g 1`, "[0 2]")
}

// §5 (COMPILE FAILURE-CLOSURE, landed 2026-07-17) — a def of a STATICALLY-COUNTED
// variadic loop region binds the region's FIRST value and spills the rest,
// exactly the interpreter's pending-forward collection (probe-pinned: the
// first-ARRIVED value satisfies the forward; `def xs (for 2 [7 8]) xs` binds
// 7). Check-mode half: SplitLoopRegionBind swaps the def's bound value for
// an ELEMENT carrier and returns the region carrier to the check stack as
// the N-1 REST residual (still the loop event's variadic out, owned by the
// existing disposition). Lowering half: the splice-at-depth OpBindGlobal
// binds stack[top-(regionN-1)] and splices it out; reads re-resolve the
// live binding via OpLookupDynScope (the top-level dynScopeRescue arm).
func TestEdgeFindingLoopCollectDefCompiles(t *testing.T) {
	// The canonical fixture family: bind + spill + read.
	mustCompileWithParity(t, `def xs (for 3 [1]) xs`, "[1 1 1]")
	mustCompileWithParity(t, `def xs (for 3 [1])`, "[1 1]")
	// Distinct per-iteration values: xs = the FIRST value, spill order kept.
	mustCompileWithParity(t, `def i0 0 def xs (for 3 [def i0 (add i0 1) i0]) xs`, "[2 3 1]")
	// Multi-value body: region = trips x body-net; xs = the deepest value.
	mustCompileWithParity(t, `def xs (for 2 [7 8]) xs`, "[8 7 8 7]")
	// The read feeds a typed downstream dispatch (element typing, not the
	// region type — the silent-miscompile risk the doc's blocker named).
	mustCompileWithParity(t, `def xs (for 3 [1]) xs add 1`, "[1 1 2]")
	// Two split binds stack their regions; each bind's depth spans only its
	// own region (the second loop sits above the first's rest).
	mustCompileWithParity(t, `def a (for 2 [1]) def b (for 2 [5]) a add b`, "[1 5 6]")
	// Double read re-resolves the live binding.
	mustCompileWithParity(t, `def xs (for 3 [1]) xs xs`, "[1 1 1 1]")
	// A const-resolved count is static (n folds to 3).
	mustCompileWithParity(t, `def n 3 def xs (for n [1]) xs`, "[1 1 1]")
	// Range form.
	mustCompileWithParity(t, `def xs (for [1 4] [i]) xs`, "[2 3 1]")
	// A HETEROGENEOUS multi-value body: the split element carrier is typed
	// from the region's FIRST-arrived value (Integer 7), NOT the loop
	// carrier's child type (which mirrors the LAST value, String "x") — so
	// the downstream `xs add 1` dispatches Integer, not String-concat (PR
	// #278 review P1-a: compiled [x 7 x 71] vs interp [x 7 x 8]).
	mustCompileWithParity(t, `def xs (for 2 [7 "x"]) xs add 1`, "[x 7 x 8]")

	// Decline fences — each keeps the compile failure, with the interpreted answer asserted alongside.
	// A DYNAMIC count: the split needs the static region size.
	mustFailToCompileWithParity(t,
		`def m {n: 3} def xs (for (m get "n") [1]) xs`,
		"consumes loop results")
	// A FILTERED name splits exactly where RecordDynBind records the event —
	// the two gates ask ONE predicate (recordsFilteredDynBind), which is the
	// fix for their having drifted apart: with the split declined, the def
	// bound the whole region instead of its first value and
	// `def _ (for [1 4] [i]) _` answered `1 2 3` against the interpreter's
	// `2 3 1` — NUR116, a silent miscompile, discharged by the flip. A root
	// `_`/`$` def records, so these compile with parity; a capitalised name
	// still records nothing and still declines (see the name-gate test).
	mustCompileWithParity(t, `def _ (for 3 [1])`, "[1 1]")
	mustCompileWithParity(t, `def _ (for 2 [7 8])`, "[8 7 8]")
	// A CONDITIONALLY-REACHED split (inside a branch arm) is declined by the
	// NestedBodyDepth gate, so the branch's analysis-only binding never
	// leaks: `if false [def xs …] [] xs` declines and the interpreter's
	// undefined_word stands (PR #278 review P1-b).
	{
		src := `if false [def xs (for 3 [1])] [] xs`
		a, _ := New()
		_, iErr := a.RunInterp(src)
		b, _ := New()
		_, cCompiled, cErr := b.RunCompiled(src)
		if noteCompileDefect(t, src, nil, cErr) {
			return
		}
		if cCompiled || codeOf(iErr) != "undefined_word" || codeOf(cErr) != "compile_failed" {
			t.Errorf("%q: want compiled-compile failure + interp undefined_word, got compiled=%v cErr=[%s] iErr=[%s]",
				src, cCompiled, codeOf(cErr), codeOf(iErr))
		}
	}
	mustFailToCompileWithParity(t,
		`def m {n: 3} def xs (for (m get "n") [1]) xs`,
		"consumes loop results")
	// LOOP-CARRIED defs GRADUATED (COMPILE FAILURE-CLOSURE §9.2a, 2026-07-17): the
	// split now admits NestedBodyDepth == LoopBodyDepth and stamps the
	// runtime depth (minus the analysis round's shadow — the historical
	// [5 0 5 0] silent-SetAt-no-op). Pinned compiling in
	// TestS9LoopCarriedVariadicStore (bytecode_s9_landing_test.go).
	mustCompileWithParity(t, `for 2 [ def acc (for 2 [5]) acc ]`, "[5 5 5 5]")
	mustCompileWithParity(t, `for 3 [ def acc (for 2 [1]) ]`, "[1 1 1]")

	// Zero-trip loops keep their existing behavior: the region is pruned,
	// the def forward-collects the NEXT token (compiles), and a read with
	// no next value is the interpreter's undefined_word (check compile failure).
	mustCompileWithParity(t, `def xs (for 0 [1]) 99`, "[]")

	// The read inside a branch arm (the former TestEmitCompileFailures row): xs is
	// the ELEMENT (Integer 0 here), so `.0` over it raises — byte-identical
	// signature_error in both engines (error parity, not a value row).
	{
		src := `def xs (for 3 [i]) if (xs.0 gt 0) [xs] [[9]]`
		a, _ := New()
		_, iErr := a.RunInterp(src)
		b, _ := New()
		_, _, cErr := b.RunCompiled(src)
		if noteCompileDefect(t, src, nil, cErr) {
			return
		}
		if iErr == nil || cErr == nil || codeOf(iErr) != codeOf(cErr) {
			t.Errorf("%q: raise parity — compiled=[%s]%v interp=[%s]%v",
				src, codeOf(cErr), cErr, codeOf(iErr), iErr)
		}
	}
}

// §6 (Stage-3 fn-value dispatch, landed 2026-07-21) — a fn body whose tail
// applies a DYNAMIC value (a Function fetched from a map at runtime) to the
// waiting frame values: the boru:fmt stylesheet driver
// `def apply fn [nd:Any Any [nd (rules get (Fmt.kind nd))]]`. Before the
// noteDynFrameReplay widening the count-mismatched residual [inert-local,
// dyn-event] declined ("fn apply: result above a literal"); now the recorder
// arms the whole-frame replay (OpCallDynFrame + RetReplay): the residual
// re-pushes in exact token order (replayForceOrder) and re-steps at the RET
// under execFnDefLiteral's own runtime rule — the fn applies exactly as the
// interpreter's pointer would, and a NON-callable value stays data so the
// RET raises the interpreter's own count error. The gate is the recorded
// trace: a body with any event AFTER the window's last producer (its
// effects would reorder behind the replay) keeps declining.
func TestEdgeFindingDynamicFnValueApplyBodyTail(t *testing.T) {
	// The stylesheet idiom: fn value fetched by a dynamic key, applied to
	// the waiting nd via `apply` — since the BROAD park (NUR073 clause 3)
	// the fetched fn is PLACED by its paren, so the explicit apply is the
	// application act (the pre-BROAD "fn steps first and forward-collects"
	// variant is the removed idiom).
	interpOnlyWithCompileFailure(t,
		`def rules {inc: ([x:Integer] => [x add 1])}
		 def app fn [[nd:Any m:Map] [Any] [nd (m get "inc") apply]]
		 app 5 rules`,
		"[6]")
	// NOT-callable runtime value: the replay leaves it as data and the RET
	// raises the interpreter's own return-count type_error (positions may
	// differ — the VM raises in the callee frame — but code and message
	// match; the differential skips ERROR rows for exactly this reason).
	{
		src := `def rules {inc: 42}
		 def app fn [[nd:Any m:Map] [Any] [nd (m get "inc")]]
		 app 5 rules`
		a, _ := New()
		_, iErr := a.RunInterp(src)
		b, _ := New()
		_, compiled, cErr := b.RunCompiled(src)
		if noteCompileDefect(t, src, nil, cErr) {
			return
		}
		if !compiled {
			t.Errorf("%q: the not-callable sibling must still compile", src)
		}
		if iErr == nil || cErr == nil || codeOf(iErr) != codeOf(cErr) {
			t.Errorf("%q: raise parity — compiled=[%s]%v interp=[%s]%v",
				src, codeOf(cErr), cErr, codeOf(iErr), iErr)
		}
		if iErr != nil && cErr != nil &&
			(!strings.Contains(cErr.Error(), "expected 1 return value(s), got 2") ||
				!strings.Contains(iErr.Error(), "expected 1 return value(s), got 2")) {
			t.Errorf("%q: count-error text — compiled=%v interp=%v", src, cErr, iErr)
		}
	}
	// A paren-PLACED fetched fn with an event after it. This used to decline
	// ("unapplied fn-value in body residual": a mid-body dynamic apply the
	// replay could not seat without reordering the print). GRADUATED
	// 2026-09-22 (NUR182, the quotation-body container reads): the paren
	// placed the fn and a fn frame never re-steps a placed value, so there
	// is no apply to seat — the residual is `[5 fn]` on both lanes, the
	// print runs where it stands, and the RET raises the interpreter's own
	// count error.
	{
		src := `def rules {inc: ([x:Integer] => [x add 1])}
		 def app fn [[nd:Any m:Map] [Any] [nd (m get "inc") print "after"]]
		 app 5 rules`
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if codeOf(errI) != "type_error" {
			t.Fatalf("%q: interpreter oracle moved: %v err=[%s]", src, gotI, codeOf(errI))
		}
		if !compiled {
			t.Errorf("%q: not compiled: %v", src, errC)
		} else {
			requireParityHead(t, src, gotC, errC, gotI, errI)
		}
	}
}
