package test

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// The `ref` word and `/v` suffix break the asymmetry between value
// bindings (where a bare name pushes the value) and fn bindings (where
// a bare name invokes). In the unified dispatch model, both `/v` and
// `ref` produce UNQUOTED Function values — these dispatch with full
// signature matching when the engine processes them, the same as a
// word lookup would. `quote (foo/v)` produces an inert Quoted Function
// value that `apply` can invoke explicitly.

// TestRefBuildsDispatchTable: two fns captured into a map by `/v`,
// retrieved by key. The map slots hold Function VALUES (unquoted —
// they're live call-sites the engine will dispatch when given args).
func TestRefBuildsDispatchTable(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def myadd fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`def mymul fn [[a:Integer b:Integer] [Integer] [a mul b]]`,
		`def ops {plus: myadd/v times: (valof mymul)}`,
		`ops`,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}
	m, _ := native.AsMap(result[0])
	if m == nil {
		t.Fatalf("expected map, got %s", result[0].Parent.String())
	}

	for _, key := range []string{"plus", "times"} {
		v, ok := m.Get(key)
		if !ok {
			t.Fatalf("ops[%q] missing", key)
		}
		if !v.Parent.Equal(core.TFunction) {
			t.Errorf("ops[%q].Parent = %s, want Function", key, v.Parent.String())
		}
		if v.Quoted {
			t.Errorf("ops[%q] is Quoted — captured Function should be unquoted (live call-site)", key)
		}
		fnDef, ok := v.Data.(core.FnDefInfo)
		if !ok {
			t.Fatalf("ops[%q] payload type = %T, want FnDefInfo", key, v.Data)
		}
		wantName := map[string]string{"plus": "myadd", "times": "mymul"}[key]
		if fnDef.Name != wantName {
			t.Errorf("ops[%q] fnDef.Name = %q, want %q", key, fnDef.Name, wantName)
		}
		if len(fnDef.OwnSigs()) == 0 {
			t.Errorf("ops[%q] captured FnDef has no Sigs — handle is hollow", key)
		}
	}
}

// TestRefMapRetrievalViaDotInvokesWithForwardArgs is the headline new
// behavior: `ops.plus 2 3` retrieves the Function value and invokes it
// against the trailing 2 and 3 via full sig matching. Postfix dispatch
// at last.
func TestRefMapRetrievalViaDotInvokesWithForwardArgs(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def myadd fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`def ops {plus: myadd/v}`,
		`ops.plus 2 3`,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1: %v", len(result), result)
	}
	got, err := core.AsInteger(result[0])
	if err != nil {
		t.Fatalf("AsInteger: %v", err)
	}
	if got != 5 {
		t.Errorf("ops.plus 2 3 = %d, want 5", got)
	}
}

// TestRefMapRetrievalAsData: when nothing follows that matches the
// captured fn's signature, the retrieved Function value sits on the
// stack as data.
func TestRefMapRetrievalAsData(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def myadd fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`def ops {plus: myadd/v}`,
		`ops.plus`,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}
	v := result[0]
	if !v.Parent.Equal(core.TFunction) {
		t.Fatalf("ops.plus type = %s, want Function", v.Parent.String())
	}
	fnDef, _ := v.Data.(core.FnDefInfo)
	if fnDef.Name != "myadd" {
		t.Errorf("ops.plus fnDef.Name = %q, want %q", fnDef.Name, "myadd")
	}
}

// TestRefSurvivesRedefinition: rebinding the underlying name doesn't
// change a map entry that captured the original fn via /v.
func TestRefSurvivesRedefinition(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def myop fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`def ops {go: myop/v}`,
		// Replace myop with multiplication instead of addition.
		`undef myop`,
		`def myop fn [[a:Integer b:Integer] [Integer] [a mul b]]`,
		// The map's captured fn still adds; the new myop multiplies.
		`ops.go 2 3`,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	got, err := core.AsInteger(result[0])
	if err != nil {
		t.Fatalf("AsInteger: %v", err)
	}
	if got != 5 {
		t.Errorf("captured ops.go (add) on 2,3 = %d, want 5 — late-binding leaked into map", got)
	}
}

// TestValofOnUndefinedNameErrors: an unbound name is the ONE compile failure
// left after `/v` became total over binding kinds, via both surface
// forms. The error must name the unbound operand — asserting only that
// something failed would pass even if the surface word itself were the
// undefined one.
func TestValofOnUndefinedNameErrors(t *testing.T) {
	for _, src := range []string{`valof nope`, `nope/v`} {
		_, err := runNativeSteps(t, nil, []string{src})
		if err == nil {
			t.Errorf("%s: expected error, got nil", src)
			continue
		}
		if !strings.Contains(err.Error(), "nope") {
			t.Errorf("%s: error %q must name the unbound operand `nope`", src, err)
		}
	}
}

// TestValOnNonFunctionBindingIsTheValue: both surfaces (`valof` and the
// `/v` suffix) are TOTAL over every binding kind. For a fn binding they
// suppress the call; for any other binding they are the identity. That
// totality is the point — it is what lets one spelling read a slot whose
// kind is not known statically (NUR085, resolved).
func TestValOnNonFunctionBindingIsTheValue(t *testing.T) {
	for _, src := range []string{`answer/v`, `valof answer`} {
		res, err := runNativeSteps(t, nil, []string{
			`def answer 42`,
			src,
		})
		if err != nil {
			t.Errorf("%s: unexpected error: %v", src, err)
			continue
		}
		if len(res) != 1 {
			t.Errorf("%s: got %v, want exactly one value", src, res)
			continue
		}
		got, gerr := core.AsInteger(res[0])
		if gerr != nil || got != 42 {
			t.Errorf("%s: got %v (err %v), want 42", src, res[0], gerr)
		}
	}
}

// TestValOnUnboundNameIsStillAnError is the negative twin: dropping the
// function-only gate did NOT make `/v` accept anything. A name with no
// binding has no value to take, and both surfaces still decline.
func TestValOnUnboundNameIsStillAnError(t *testing.T) {
	for _, src := range []string{`nope/v`, `valof nope`} {
		_, err := runNativeSteps(t, nil, []string{src})
		if err == nil {
			t.Errorf("%s: expected an error for an unbound name, got nil", src)
			continue
		}
		if !strings.Contains(err.Error(), "not bound") &&
			!strings.Contains(err.Error(), "undefined word") {
			t.Errorf("%s: error=%q, want an unbound/undefined report", src, err.Error())
		}
	}
}

// --- direct dispatch of unquoted Function values --------------------

// TestRefSuffixHoldsArgsUndispatched: `/v` is a pure reference and does
// NOT dispatch — it advances the pointer, so `myadd/v 2 3` holds the
// function and leaves the args untouched: [Function, 2, 3]. The call is
// written `myadd 2 3` (bare word) or via `apply` (TestApplyOnQuotedCapture).
func TestRefSuffixHoldsArgsUndispatched(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def myadd fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`myadd/v 2 3`,
	})
	if err != nil {
		t.Fatalf("ref: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d results, want 3 [Function 2 3]: %v", len(result), result)
	}
	if !result[0].Parent.Equal(core.TFunction) {
		t.Errorf("result[0].Parent=%s, want Function (held, not dispatched)", result[0].Parent.String())
	}
	if a, _ := core.AsInteger(result[1]); a != 2 {
		t.Errorf("result[1]=%v, want 2 (arg not consumed)", result[1])
	}
	if b, _ := core.AsInteger(result[2]); b != 3 {
		t.Errorf("result[2]=%v, want 3 (arg not consumed)", result[2])
	}
	// Call path: the bare word still dispatches.
	bare, err := runNativeSteps(t, nil, []string{
		`def myadd fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`myadd 2 3`,
	})
	if err != nil {
		t.Fatalf("bare: %v", err)
	}
	if got, _ := core.AsInteger(bare[0]); got != 5 {
		t.Errorf("myadd 2 3 = %d, want 5", got)
	}
}

// TestInlineFnLiteralDispatchesWithStackArgs: a bare `(fn [...])`
// expression dispatches with preceding stack args. Anonymous fns
// don't ship compiled Signatures (no name → no InstallFnDef pass),
// so forward collection isn't available — but the FnSig stack-match
// path picks them up.
func TestInlineFnLiteralDispatchesWithStackArgs(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`2 3 (fn [[a:Integer b:Integer] [Integer] [a add b]]) apply`,
	})
	if err != nil {
		t.Fatalf("inline fn: %v", err)
	}
	got, _ := core.AsInteger(result[0])
	if got != 5 {
		t.Errorf("inline fn dispatch = %d, want 5", got)
	}
}

// --- apply: invokes Quoted Function values explicitly --------------

// TestApplyOnQuotedCapture: a Quoted Function is inert until `apply`
// flips the Quoted flag. The engine then dispatches via full sig
// matching against preceding stack args. We use `(quote (myadd/v))`
// to evaluate /v first (producing an unquoted Function), then wrap
// in `quote` to mark it as data.
func TestApplyOnQuotedCapture(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def myadd fn [[a:Integer b:Integer] [Integer] [a add b]]`,
		`2 3 (quote (myadd/v)) apply`,
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, _ := core.AsInteger(result[0])
	if got != 5 {
		t.Errorf("2 3 (quote (myadd/v)) apply = %d, want 5", got)
	}
}

// TestApplyErrorsOnNonFunction: type check still rejects non-fn
// values.
func TestApplyErrorsOnNonFunction(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`42 apply`,
	})
	if err == nil {
		t.Fatal("expected error applying to Integer, got nil")
	}
}

// TestRefMatchesSlashR pins that the `ref` word and the `/v` suffix are the
// SAME operation in every position: both leave an inert (but unquoted)
// Function reference at the call site, and that reference dispatches only
// when it is re-stepped elsewhere (unwrapped from a paren, retrieved from a
// map). In particular a bare reference to a 0-arg fn does NOT auto-fire —
// `val f` and `f/v` both hold the function, and since the BROAD park
// (NUR073 clause 3) a paren HOLDS it identically: `(valof f)` ≡ `valof f`.
// This remains the guard that `valof` and `/v` never diverge from each
// other, whatever the paren semantics of the day.
func TestValMatchesSlashV(t *testing.T) {
	cases := []struct {
		name      string
		refStep   string // ref-word form
		slashStep string // /v-suffix form
		wantFn    bool   // true: top is an inert Function; false: top is the called result
	}{
		{"bare 0-arg held", `valof f`, `f/v`, true},
		{"paren 0-arg places (BROAD)", `(valof f)`, `(f/v)`, true},
	}
	def0 := `def f fn [[] [Integer] [42]]`
	for _, c := range cases {
		refRes, refErr := runNativeSteps(t, nil, []string{def0, c.refStep})
		rRes, rErr := runNativeSteps(t, nil, []string{def0, c.slashStep})
		if refErr != nil || rErr != nil {
			t.Fatalf("%s: ref err=%v, /v err=%v", c.name, refErr, rErr)
		}
		if len(refRes) != 1 || len(rRes) != 1 {
			t.Fatalf("%s: ref=%v /v=%v, want 1 value each", c.name, refRes, rRes)
		}
		refIsFn := refRes[0].Parent.Equal(core.TFunction)
		rIsFn := rRes[0].Parent.Equal(core.TFunction)
		if refIsFn != rIsFn {
			t.Errorf("%s: ref-is-fn=%v but /v-is-fn=%v — ref and /v must match", c.name, refIsFn, rIsFn)
		}
		if refIsFn != c.wantFn {
			t.Errorf("%s: got fn=%v, want fn=%v (ref=%v)", c.name, refIsFn, c.wantFn, refRes[0])
		}
	}

	// A map-stored reference dispatches identically whether built with
	// `valof` or `/v`.
	for _, mk := range []struct{ name, slot string }{
		{"ref", `(valof f1)`},
		{"/v", `f1/v`},
	} {
		res, err := runNativeSteps(t, nil, []string{
			`def f1 fn [[n:Integer] [Integer] [n add 1]]`,
			`def ops {g: ` + mk.slot + `}`,
			`ops.g 5`,
		})
		if err != nil {
			t.Fatalf("map %s: %v", mk.name, err)
		}
		n, err := native.AsInteger(res[len(res)-1])
		if err != nil || n != 6 {
			t.Errorf("map %s: ops.g 5 = %v (err %v), want 6", mk.name, res, err)
		}
	}
}

// TestRefInListHoldsFunctionAnyArity pins the pure-reference contract:
// `/v` advances the pointer and never dispatches, so a function
// referenced inside a list is held as data regardless of arity — a 0-arg
// fn is NOT fired in place. (To run it, call the bare word, `apply` it,
// or access it as a member where `get` brings the value live.)
func TestRefInListHoldsFunctionAnyArity(t *testing.T) {
	cases := []struct{ name, def string }{
		{"0-arg", `def f fn [[] [Integer] [42]]`},
		{"2-arg", `def f fn [[a:Integer b:Integer] [Integer] [a add b]]`},
	}
	for _, c := range cases {
		res, err := runNativeSteps(t, nil, []string{c.def, `[f/v]`})
		if err != nil {
			t.Fatalf("%s [f/v]: %v", c.name, err)
		}
		l, _ := core.AsList(res[0])
		if l.Len() != 1 || !l.Get(0).Parent.Equal(core.TFunction) {
			t.Errorf("%s [f/v] = %v, want [Function] (held, not dispatched)", c.name, res[0])
		}
	}
}
