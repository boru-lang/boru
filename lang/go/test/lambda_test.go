package test

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"

	"github.com/boru-lang/boru/lang/go/native"
)

// Lambda syntax `a => b` is sugar for `a afn b`. The `=>` token parses
// to a structural sugar marker (ADR-012: the parser emits no names);
// the engine lowers the marker to the word the SugarLambda role binds
// (`afn` in this language). `afn` is a normal native word with
// signature [Any Any |] that constructs an anonymous Function value
// with one signature, Returns=[Any], and the Anonymous flag set so
// check-mode can route through AnalyseFnBody.

// TestArrowTokenAliasesAfn — the `=>` token parses to the same value
// stream as the literal `afn` word except at the arrow position,
// which carries the SugarLambda marker instead of the word. Typed-
// param shorthand must be list-wrapped (`[x:Integer]`); a bare
// `x:Integer` at top level starts an implicit map and the rest of the
// expression collapses into the map's value position.
func TestArrowTokenAliasesAfn(t *testing.T) {
	a, err := parser.Parse("[x:Integer] => [x add 1]")
	if err != nil {
		t.Fatalf("parse `=>`: %v", err)
	}
	b, err := parser.Parse("[x:Integer] afn [x add 1]")
	if err != nil {
		t.Fatalf("parse `afn`: %v", err)
	}
	if len(a) != len(b) {
		t.Fatalf("token count: => produced %d, afn produced %d", len(a), len(b))
	}
	for i := range a {
		if info, ok := core.AsSugar(a[i]); ok {
			// The arrow position: a lambda marker where the literal
			// spelling has word(afn).
			if info.Kind != core.SugarLambda {
				t.Errorf("token[%d]: marker kind %q, want lambda", i, info.Kind)
			}
			if b[i].String() != "word(afn)" {
				t.Errorf("token[%d]: afn spelling has %q at the arrow position", i, b[i].String())
			}
			continue
		}
		if core.ParentOf(&a[i]) != core.ParentOf(&b[i]) {
			t.Errorf("token[%d] parent differs: => %s, afn %s", i, core.ParentOf(&a[i]).String(), core.ParentOf(&b[i]).String())
		}
		if a[i].String() != b[i].String() {
			t.Errorf("token[%d] string differs: => %q, afn %q", i, a[i].String(), b[i].String())
		}
	}
}

// A bare-Word body dispatches as the engine walks past it during
// forward collection, so the body must be wrapped — a list `[x]`
// or paren that resolves to a value. The single-value body rule
// allows non-list bodies only when the value is itself a literal
// (Integer, String, etc.) that doesn't dispatch.
func TestLambdaIdentity(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`7 ([x:Any] => [x]) apply`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1: %v", len(result), result)
	}
	n, _ := core.AsInteger(result[0])
	if n != 7 {
		t.Errorf("identity lambda → %d, want 7", n)
	}
}

// Literal body (single non-Word value) — afn captures it directly.
func TestLambdaLiteralBody(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`"ignored" ([x:Any] => 42) apply`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1: %v", len(result), result)
	}
	n, _ := core.AsInteger(result[0])
	if n != 42 {
		t.Errorf("literal-body lambda → %d, want 42", n)
	}
}

func TestLambdaForwardApply(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`5 ([x:Integer] => [x add 1]) apply`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1: %v", len(result), result)
	}
	n, _ := core.AsInteger(result[0])
	if n != 6 {
		t.Errorf("(x:Integer => [x add 1]) 5 → %d, want 6", n)
	}
}

func TestLambdaTwoArgs(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`2 3 ([x:Integer y:Integer] => [x add y]) apply`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1: %v", len(result), result)
	}
	n, _ := core.AsInteger(result[0])
	if n != 5 {
		t.Errorf("got %d, want 5", n)
	}
}

func TestLambdaAfnAlias(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`5 ([x:Integer] afn [x add 1]) apply`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1: %v", len(result), result)
	}
	n, _ := core.AsInteger(result[0])
	if n != 6 {
		t.Errorf("got %d, want 6", n)
	}
}

func TestLambdaDefBinding(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def double ([x:Integer] => [x mul 2])`,
		`double 5`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1: %v", len(result), result)
	}
	n, _ := core.AsInteger(result[0])
	if n != 10 {
		t.Errorf("got %d, want 10", n)
	}
}

func TestLambdaProducesAnonymousFunction(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`[x:Integer] => [x add 1]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}
	v := result[0]
	if !core.ParentOf(&v).Equal(core.TFunction) {
		t.Fatalf("Parent = %s, want Function", core.ParentOf(&v).String())
	}
	fnDef, ok := core.FnDefOf(core.DataOf(&v))
	if !ok {
		t.Fatalf("payload type = %T, want FnDefInfo", core.DataOf(&v))
	}
	if !fnDef.Anonymous {
		t.Errorf("FnDefInfo.Anonymous = false, want true (afn must mark its output)")
	}
	ownSigs := fnDef.OwnSigs()
	if len(ownSigs) != 1 {
		t.Errorf("got %d sigs, want 1", len(ownSigs))
	}
	if len(ownSigs[0].Returns) != 1 || ownSigs[0].Returns[0] != core.TAny {
		t.Errorf("Returns = %v, want [Any]", ownSigs[0].Returns)
	}
}

// fnHandler-produced Functions must NOT carry the Anonymous flag —
// only afn sets it. This pin guards against accidental cross-wiring.
func TestFnNamedIsNotAnonymous(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`fn [[x:Integer] [Integer] [x add 1]]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	v := result[0]
	fnDef, ok := core.FnDefOf(core.DataOf(&v))
	if !ok {
		t.Fatalf("payload type = %T, want FnDefInfo", core.DataOf(&v))
	}
	if fnDef.Anonymous {
		t.Errorf("fn-produced FnDefInfo.Anonymous = true, want false")
	}
}

// Mute the unused-import shim if native isn't referenced elsewhere
// in this file via build-time use.
var _ = native.NewTop

// Check-mode inference: an anonymous lambda's return carrier reflects
// the body's actual output type, not the conservative static
// `Returns=[Any]`. The Anonymous flag on FnDefInfo routes dispatch
// through check.AnalyseFnBody instead of body-splice + ReturnCheck.
func runCheckMode(t *testing.T, src string) []native.Value {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	native.Register(reg)
	reg.Check.Mode = true
	defer func() { reg.Check.Mode = false }()

	e := native.NewTop(reg)
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := e.Run(vals)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return out
}

func TestLambdaCheckModeInfersInteger(t *testing.T) {
	out := runCheckMode(t, `5 ([x:Integer] => [x add 1]) apply`)
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1: %v", len(out), out)
	}
	v := out[0]
	if !core.IsCarrier(&v) {
		t.Errorf("expected carrier in check mode, got %v", v)
	}
	if !core.ParentOf(&v).Equal(core.TInteger) {
		t.Errorf("carrier Parent = %s, want Integer", core.ParentOf(&v).String())
	}
}

func TestLambdaCheckModeInferenceFlowsThroughDef(t *testing.T) {
	out := runCheckMode(t, `def double ([x:Integer] => [x mul 2])
double 5`)
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1: %v", len(out), out)
	}
	v := out[0]
	if !core.IsCarrier(&v) {
		t.Errorf("expected carrier in check mode, got %v", v)
	}
	if !core.ParentOf(&v).Equal(core.TInteger) {
		t.Errorf("carrier Parent = %s, want Integer", core.ParentOf(&v).String())
	}
}

// Without the Anonymous flag, a fn-produced FnDef with explicit
// Returns=[Integer] still produces an Integer carrier — but via the
// standard sig.Returns path, not AnalyseFnBody. The point of the
// Anonymous-routed path is that the static Returns=[Any] doesn't
// erase the inferred type.
func TestLambdaCheckModeStaticAnyIsIgnored(t *testing.T) {
	out := runCheckMode(t, `([x:Integer] => [x add 1])`)
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1: %v", len(out), out)
	}
	v := out[0]
	// The lambda itself sits on the stack as a Function carrier or
	// concrete value; the inspect-static Returns=[Any] doesn't matter
	// here because we haven't called the lambda yet — this just
	// confirms it constructs without error.
	if !core.ParentOf(&v).Equal(core.TFunction) {
		t.Errorf("constructed lambda Parent = %s, want Function", core.ParentOf(&v).String())
	}
}
