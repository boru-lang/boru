// Equivalence tests for the kernel-level eng/go/stackform package
// against the language-layer registry. The contract under test:
//
//	stackform.Eval(reg, stackform.Compile(reg, src)) == native.Engine.Run(src)
//
// for representative boru programs. Lives here in lang/go/test rather
// than eng/go/stackform_test because the stackform package can't
// import the language layer (upward dependency) but the test needs
// real native words (math, comparison, etc.) to exercise.
package test

import (
	"errors"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/stackform"
	parser "github.com/boru-lang/boru/parser/go"
)

func stackformReg(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	r.SetParseFunc(parser.Parse)
	return r
}

// equivalentRun runs `src` through both:
//
//  1. A normal NewTop engine — the "direct" baseline.
//  2. stackform.Compile to record a StackForm, then stackform.Eval
//     to replay it.
//
// Both should produce the same final stack. Differences are
// surfaced as test failures with both stacks in the message.
func equivalentRun(t *testing.T, src string) {
	t.Helper()
	r := stackformReg(t)
	tokens, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}

	// Direct run on a fresh engine for the baseline.
	directReg := stackformReg(t)
	direct, err := native.NewTop(directReg).Run(append([]core.Value(nil), tokens...))
	if err != nil {
		t.Fatalf("direct run %q: %v", src, err)
	}

	// Compile + Eval round-trip.
	_, form, err := stackform.Compile(r, append([]core.Value(nil), tokens...))
	if err != nil {
		t.Fatalf("compile %q: %v", src, err)
	}
	round, err := stackform.Eval(r, form)
	if err != nil {
		t.Fatalf("eval %q: %v\n  form: %s", src, err, stackform.Pretty(form))
	}

	if !stacksEqual(direct, round) {
		t.Errorf("%q: stacks differ\n  direct=%v\n  round =%v\n  form=%s",
			src, direct, round, stackform.Pretty(form))
	}
}

func stacksEqual(a, b []core.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		// Functions are checked FIRST and never fall through to the plain deq
		// below. Ordering is load-bearing: `deq` returns true for two
		// Functions differing only in Quoted, so a leading DeepEqual would
		// short-circuit past the one property this suite most needs to see.
		if isFn(a[i]) && isFn(b[i]) {
			if fnValuesEqual(a[i], b[i]) {
				continue
			}
			return false
		}
		if core.DeepEqual(a[i], b[i]) {
			continue
		}
		return false
	}
	return true
}

func isFn(v core.Value) bool {
	return v.Parent != nil && v.Parent.Equal(core.TFunction)
}

// fnValuesEqual compares two Function values for round-trip purposes.
//
// Since NUR031's fix landed, `deq` handles functions properly: it is
// reflexive, and it discriminates closure environments — verified, two
// closures capturing `x=1` and `x=2` are NOT DeepEqual. So the structural
// half needs nothing hand-written any more, and the earlier canon-based
// fallback here is gone with it.
//
// What deq still does not see is `Quoted`, and that omission is defensible on
// its own terms — the flag is a transient marker, not part of a value's
// identity. It makes deq the wrong SOLE test here all the same, because that
// flag is precisely what the replay borrows to keep a Function inert and has
// to give back: `/v` parks positionally and stamps nothing, so a mark left in
// place returns a permanently-inert copy of a live value. Canon cannot stand
// in either — its Function branch renders name, params, returns and body, and
// never the flag.
//
// The HOME is normalised away. A fn value carries the registry that minted it
// and deq compares homes by module, but this harness runs the direct baseline
// and the replay on two separate registries by construction (equivalentRun),
// so a same-text fn from each is two modules to deq and one function to this
// round trip. Stripping the home leaves everything the round trip is about.
func fnValuesEqual(a, b core.Value) bool {
	return a.Quoted == b.Quoted && core.DeepEqual(stripFnHome(a), stripFnHome(b))
}

func stripFnHome(v core.Value) core.Value {
	if fd, ok := v.Data.(core.FnDefInfo); ok {
		fd.Registry = nil
		v.Data = fd
	}
	return v
}

// TestStackFormEquivalence_Arithmetic covers integer + decimal
// arithmetic in both forward and stack form. These are the smallest
// programs the compiler-via-recorder should handle.
func TestStackFormEquivalence_Arithmetic(t *testing.T) {
	for _, src := range []string{
		`1 2 add`,
		`add 1 2`,
		`3 4 mul`,
		`mul 3 4`,
		`10 3 sub`,
		`sub 10 3`,
		`5 2 mul 7 add`,
		`(5 2 mul) 7 add`,
		`100 3 div`,
		`1.5 2.5 add`,
	} {
		t.Run(src, func(t *testing.T) {
			equivalentRun(t, src)
		})
	}
}

// TestStackFormEquivalence_Comparisons + boolean ops.
func TestStackFormEquivalence_Comparisons(t *testing.T) {
	for _, src := range []string{
		`5 3 gt`,
		`5 3 lt`,
		`5 5 eq`,
		`true false and`,
		`true false or`,
		`true not`,
	} {
		t.Run(src, func(t *testing.T) {
			equivalentRun(t, src)
		})
	}
}

// TestStackFormEquivalence_Strings covers string concat / case.
func TestStackFormEquivalence_Strings(t *testing.T) {
	for _, src := range []string{
		`"hello" upper`,
		`"WORLD" lower`,
		`"foo" "bar" add`,
		`add "foo" "bar"`,
	} {
		t.Run(src, func(t *testing.T) {
			equivalentRun(t, src)
		})
	}
}

// TestStackFormEquivalence_StackOps covers dup, swap, drop —
// pure stack manipulation that the recorder needs to track even
// though no math is involved.
func TestStackFormEquivalence_StackOps(t *testing.T) {
	for _, src := range []string{
		`1 dup`,
		`1 2 swap`,
		`1 2 3 drop`,
		`1 2 over`,
		`1 dup mul`, // 1²
		`3 dup mul`, // 9
	} {
		t.Run(src, func(t *testing.T) {
			equivalentRun(t, src)
		})
	}
}

// TestStackFormEquivalence_Lists covers list literals + simple list ops.
func TestStackFormEquivalence_Lists(t *testing.T) {
	for _, src := range []string{
		`[1 2 3]`,
		`[1 2 3] size`,
	} {
		t.Run(src, func(t *testing.T) {
			equivalentRun(t, src)
		})
	}
}

// TestStackFormPrettyRoundTrip checks Pretty produces output that
// the parser can re-ingest into an equivalent StackForm. This is a
// weaker property than Compile-Eval equivalence — it's about the
// human-readable rendering being faithful.
func TestStackFormPrettyRoundTrip(t *testing.T) {
	for _, src := range []string{
		`1 2 add`,
		`5 3 sub 2 mul`,
		`"foo" "bar" add`,
		`true false and`,
	} {
		t.Run(src, func(t *testing.T) {
			r := stackformReg(t)
			tokens, err := parser.Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			_, form1, err := stackform.Compile(r, append([]core.Value(nil), tokens...))
			if err != nil {
				t.Fatal(err)
			}
			pretty := stackform.Pretty(form1)
			t.Logf("Pretty(%q) = %q", src, pretty)
			tokens2, err := parser.Parse(pretty)
			if err != nil {
				t.Fatalf("re-parse pretty output %q: %v", pretty, err)
			}
			r2 := stackformReg(t)
			_, form2, err := stackform.Compile(r2, tokens2)
			if err != nil {
				t.Fatalf("re-compile: %v", err)
			}
			if !stackform.Equal(form1, form2) {
				t.Errorf("round-trip not stable\n  first  : %s\n  pretty : %s\n  second : %s",
					stackform.Pretty(form1), pretty, stackform.Pretty(form2))
			}
		})
	}
}

// TestStackFormEquivalence_UserFunctions is the case the suite above was
// missing entirely: every one of its programs calls a NATIVE word, so the
// contract went untested for a boru `fn` and the recorder broke it for
// years without a red test.
//
// It broke two ways at once. The frame-splice OVER-COUNT: OnCall was handed
// `len(results)`, which for a boru fn is the spliced fn-frame skeleton (eight
// tokens for a 1-arg/1-return fn), not the return count — so the recorder's
// skip counter swallowed unrelated later literals, and `z 99` recorded a form
// that dropped the 99. And the BODY LEAK: the callee's own dispatches were
// recorded at top level, so the form re-ran fragments of the body after the
// call had already run it.
//
// The trailing-literal rows are the over-count's direct witness: a value
// AFTER the call is exactly what an inflated skip count eats.
func TestStackFormEquivalence_UserFunctions(t *testing.T) {
	const z = `def z fn [[] [Integer] [42]] `
	const inc = `def inc fn [[n:Integer] [Integer] [n add 1]] `

	for _, src := range []string{
		z + `z`,
		z + `z 99`,          // over-count witness: 0-arg
		inc + `inc 5`,       // body leak witness: add/__pa/undef leaked
		inc + `(inc 5) 99`,  // over-count witness: 1-arg
		inc + `inc (inc 5)`, // nesting
		inc + `def twice fn [[n:Integer] [Integer] [inc (inc n)]] twice 5`,
		`def p fn [[] [Integer Integer] [1 2]] p`, // multi-return
		`def fact fn [[n:Integer] [Integer] [if (n lte 1) [1] [n mul (fact (n sub 1))]]] fact 4`,
		// A fn handed to a fn as a Function param — the callback shape every
		// boru library uses — with the returned reference held by /v.
		inc + `def ap fn [[f:Function n:Integer] [Integer] [f n]] ap inc/v 5`,
		z + `def h fn [[f:Function] [Any] [f/v]] h z/v 99`,
		// A `/v` reference left as the RESIDUAL, not consumed by a later call.
		// Flatten has to mark a Function PushLit Quoted to stop the replay
		// dispatching it, and Quoted is STICKY where `/v` is positional — so
		// without Eval undoing the mark this row returns a permanently-inert
		// copy of a value the direct run returns live. `canon` omits the flag,
		// so only stacksEqual's explicit check sees it (PR #378 review, P1).
		inc + `inc/v`,
		z + `z/v`,
		// A CLOSURE as the residual: same shape, but the value also carries a
		// captured environment. canon deliberately omits Captured too, so this
		// row is only meaningful because stacksEqual compares it (P2).
		`def mk fn [[x:Integer] [Function] [([y:Integer] => [x add y])]] (mk 5)`,
	} {
		t.Run(src, func(t *testing.T) {
			equivalentRun(t, src)
		})
	}
}

// TestStackFormRefusesFunctionValueApplication pins the NEGATIVE half: a
// program the recorder cannot capture faithfully must be REFUSED, never
// replayed to a different answer than it was recorded from.
//
// Applying a function VALUE — an inline lambda, or a fn read out of a
// container — is not expressible: `Call{Name, Arity}` re-invokes by name and
// does not consume a receiver, while an application consumes the fn value the
// stack already holds. Recording it as a Call would strand that value; the op
// vocabulary needs an apply-style Op it does not have (NUR077).
//
// Before this, these silently evaluated to the FUNCTION rather than its
// result — the same class of quiet wrongness the over-count caused, and the
// reason the PBT shrinker could report a counterexample its generator cannot
// produce.
func TestStackFormRefusesFunctionValueApplication(t *testing.T) {
	for _, src := range []string{
		// The in-group spelling: the lambda dispatches at the pointer with
		// its forward arg. (The old row `([n:Integer] => [n add 1]) 5`
		// stopped being an application under the BROAD park — the paren
		// places the lambda and the 5 rides — so it records as data and
		// replays fine; NUR073 clause 3.)
		`(n:Integer => [n add 1] 5)`,
		`def fs [ fn [[n:Integer] [Integer] [n add 1]] ] ((fs get 0) 5)`,
		`def m {f: (fn [[n:Integer] [Integer] [n add 1]])} (m.f 5)`,
	} {
		t.Run(src, func(t *testing.T) {
			r := stackformReg(t)
			tokens, err := parser.Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			_, form, err := stackform.Compile(r, tokens)
			if err != nil {
				t.Fatalf("compile %q: %v", src, err)
			}
			if err := stackform.Replayable(form); !errors.Is(err, stackform.ErrUnnamedApply) {
				t.Errorf("%q: Replayable = %v, want ErrUnnamedApply\n  form: %s",
					src, err, stackform.Pretty(form))
			}
			if _, err := stackform.Eval(stackformReg(t), form); !errors.Is(err, stackform.ErrUnnamedApply) {
				t.Errorf("%q: Eval = %v, want it to refuse rather than replay", src, err)
			}
		})
	}

	// POSITIVE control: an ordinary named call in the same suite must stay
	// replayable, so the refusal cannot silently widen to everything.
	r := stackformReg(t)
	tokens, err := parser.Parse(`def inc fn [[n:Integer] [Integer] [n add 1]] inc 5`)
	if err != nil {
		t.Fatal(err)
	}
	_, form, err := stackform.Compile(r, tokens)
	if err != nil {
		t.Fatal(err)
	}
	if err := stackform.Replayable(form); err != nil {
		t.Errorf("a named fn call must stay replayable, got %v", err)
	}
}

// TestStackFormLiteralAccountingExact pins the recorder's skip accounting
// against a Function-valued paren survivor — design/FN-VALUE-OPEN-WORK.0.md
// §5.2, the same class as the frame-skeleton over-count fixed in 9bffdd3.
//
// A survivor earns a skip credit only if the re-step would fire OnPushLit for
// it. A Function value never does (execFnDefLiteral dispatches it, or the
// ADR-016 0-arg gate steps past it), so crediting one left an unspendable
// skip that silently SWALLOWED the next real literal: `(z/v) 777` recorded
// without its 777 at all, and each extra paren level ate another.
//
// The assertion is on the LITERALS the form carries, not on Eval's stack:
// these shapes still strand the function (NUR077's own defect, which only the
// apply Op can close), so equivalentRun cannot be used yet. Literal
// accounting, though, must already be exact — and a form that has silently
// dropped operands is the thing the shrinker would trust.
func TestStackFormLiteralAccountingExact(t *testing.T) {
	const zdef = `def z fn [[] [Integer] [42]] `
	const incdef = `def inc fn [[n:Integer] [Integer] [n add 1]] `
	for _, c := range []struct {
		src  string
		want []int64 // the integer literals the form must carry, in order
	}{
		// CONTROL: a non-function paren result — the balance was always exact
		// here, and must stay so.
		{`(1 add 2) 777`, []int64{1, 2, 777}},
		// One Function survivor. Before the fix this recorded NO integer at
		// all: the surplus credit ate the 777.
		{zdef + `(z/v) 777`, []int64{777}},
		{zdef + `(z/v) 777 888`, []int64{777, 888}},
		// Every extra paren level added another unspendable credit, so depth
		// is the regression that matters most.
		{zdef + `((z/v)) 777 888`, []int64{777, 888}},
		{zdef + `(((z/v))) 777 888 999`, []int64{777, 888, 999}},
		// The APPLIED shape: the function's own ARGUMENT literal was the one
		// dropped here — the form kept 777 but lost the 5 it is called with.
		{incdef + `(inc/v) 5 777`, []int64{5, 777}},
	} {
		t.Run(c.src, func(t *testing.T) {
			r := stackformReg(t)
			tokens, err := parser.Parse(c.src)
			if err != nil {
				t.Fatal(err)
			}
			_, form, err := stackform.Compile(r, tokens)
			if err != nil {
				t.Fatalf("compile %q: %v", c.src, err)
			}
			got := formIntLiterals(form)
			if len(got) != len(c.want) {
				t.Fatalf("%q: integer literals = %v, want %v\n  form: %s",
					c.src, got, c.want, stackform.Pretty(form))
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("%q: integer literals = %v, want %v\n  form: %s",
						c.src, got, c.want, stackform.Pretty(form))
				}
			}
		})
	}
}

// formIntLiterals collects the Integer PushLit payloads a form carries, in
// order. Integers alone keep the expectations readable, and they isolate
// exactly what the over-count destroyed: a fn definition pushes its body as
// one LIST op, so `fn z`'s 42 and `inc`'s 1 are inside that payload and never
// appear here — every integer below is a program literal.
func formIntLiterals(form *stackform.StackForm) []int64 {
	var out []int64
	for _, op := range form.Ops {
		pl, ok := op.(stackform.PushLit)
		if !ok {
			continue
		}
		if n, err := pl.V.AsConcreteInteger(); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// TestStackFormFunctionArgNotDoubleRecorded is the DUAL of the literal
// accounting above, and guards the hazard that fix first walked into.
//
// A paren collapsed on behalf of a pending forward is not re-stepped by the
// main loop: its survivor becomes the call's ARGUMENT. So a Function survivor
// there is collected, not dispatched, and the collection hook fires OnPushLit
// for it — meaning it DOES need its skip credit, where a main-loop survivor
// does not. Withholding it recorded the function twice (`fn z … fn z g`) and
// replay grew an extra operand.
//
// These round-trip exactly, so equivalentRun is the assertion: it fails on a
// duplicated operand as surely as on a dropped one.
func TestStackFormFunctionArgNotDoubleRecorded(t *testing.T) {
	const zdef = `def z fn [[] [Integer] [42]] `
	for _, src := range []string{
		// A paren'd reference into a Function slot, with a literal following.
		zdef + `def g fn [[f:Function] [Any] [7]] g (z/v) 777`,
		// The same call with no trailing literal.
		zdef + `def g fn [[f:Function] [Any] [7]] g (z/v)`,
		// CONTROL: the unparenthesised spelling of the same call, which never
		// went through stepCloseParen at all.
		zdef + `def g fn [[f:Function] [Any] [7]] g z/v 777`,
	} {
		t.Run(src, func(t *testing.T) { equivalentRun(t, src) })
	}
}
