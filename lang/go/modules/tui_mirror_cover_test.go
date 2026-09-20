package modules

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
)

// Direct unit coverage for the boru:tui check-mode mirrors. module-tui.tsv
// §§3-6 drive the FLAGGED shapes end to end; these tests own the arms no
// row reaches — the gate's declines, the policy-denied declines, and the
// residual each mirror hands back.

// tuiMirrorMap builds a concrete options map from alternating key/value
// pairs, so a test reads as the boru literal it stands for.
func tuiMirrorMap(kv ...any) native.Value {
	m := native.NewOrderedMap()
	for i := 0; i+1 < len(kv); i += 2 {
		m.Set(kv[i].(string), kv[i+1].(native.Value))
	}
	return native.NewMap(m)
}

func TestTuiOpenMirrorArms(t *testing.T) {
	rf := tuiOpenMirror()

	// The flagged shapes: a non-Boolean alt-screen, and the §11.7
	// reservation declining the inline tier. Both carry the handler's own
	// code — `unsupported` is deliberately NOT tui_error.
	r := vaultMirrorRegistry(t)
	out := rf([]native.Value{tuiMirrorMap("alt-screen", native.NewInteger(5))}, r)
	if len(out) != 1 || !out[0].Parent.Equal(TTerminal) {
		t.Fatalf("open residual = %v, want one Terminal carrier", out)
	}
	if len(r.Check.Diagnostics) != 1 || r.Check.Diagnostics[0].Code != "tui_error" {
		t.Fatalf("a non-Boolean alt-screen must flag tui_error, got %+v", r.Check.Diagnostics)
	}

	r2 := vaultMirrorRegistry(t)
	rf([]native.Value{tuiMirrorMap("alt-screen", native.NewBoolean(false))}, r2)
	if len(r2.Check.Diagnostics) != 1 || r2.Check.Diagnostics[0].Code != "unsupported" {
		t.Fatalf("alt-screen: false must flag unsupported, got %+v", r2.Check.Diagnostics)
	}

	// A bad title reaches the SECOND option reader — the arm the mouse
	// check short-circuits past when it fails first.
	r3 := vaultMirrorRegistry(t)
	rf([]native.Value{tuiMirrorMap("title", native.NewInteger(1))}, r3)
	if len(r3.Check.Diagnostics) != 1 || r3.Check.Diagnostics[0].Code != "tui_error" {
		t.Fatalf("a non-String title must flag tui_error, got %+v", r3.Check.Diagnostics)
	}

	// Valid options are silent: what remains is the backend acquisition,
	// which is host state and not the mirror's to decide.
	r4 := vaultMirrorRegistry(t)
	rf([]native.Value{tuiMirrorMap("mouse", native.NewBoolean(true), "title", native.NewString("t"))}, r4)
	if len(r4.Check.Diagnostics) != 0 {
		t.Fatalf("valid open options must stay silent, got %+v", r4.Check.Diagnostics)
	}

	// An operand that does not inhabit the Map slot reaches this ReturnsFn
	// only through union-combo expansion; the run never routes it here, so
	// the mirror declines and leaves the compile failure to dispatch.
	r5 := vaultMirrorRegistry(t)
	rf([]native.Value{native.NewInteger(1)}, r5)
	if len(r5.Check.Diagnostics) != 0 {
		t.Fatalf("an off-type operand must be left to dispatch, got %+v", r5.Check.Diagnostics)
	}

	// A MAP-SHAPED operand that AsMap cannot expose is the opposite case:
	// the structural subtypes share Parent=TMap and satisfy the signature,
	// so the run reaches the handler and raises this exact error.
	r7 := vaultMirrorRegistry(t)
	rf([]native.Value{{Parent: native.TMap, Data: native.RecordTypeInfo{}}}, r7)
	if len(r7.Check.Diagnostics) != 1 || r7.Check.Diagnostics[0].Detail != "open: options must be a Map" {
		t.Fatalf("an unreadable map-shaped operand must flag, got %+v", r7.Check.Diagnostics)
	}

	// A bare type node as an option VALUE is decided, not unknown.
	r8 := vaultMirrorRegistry(t)
	rf([]native.Value{tuiMirrorMap("alt-screen", native.NewTypeLiteral(native.TNone))}, r8)
	if len(r8.Check.Diagnostics) != 1 || r8.Check.Diagnostics[0].Detail != "open: alt-screen: must be a Boolean" {
		t.Fatalf("a None alt-screen must flag, got %+v", r8.Check.Diagnostics)
	}

	// A computed option value closes the gate — the runtime value decides.
	r6 := vaultMirrorRegistry(t)
	rf([]native.Value{tuiMirrorMap("alt-screen", core.NewCarrier(core.TBoolean))}, r6)
	if len(r6.Check.Diagnostics) != 0 {
		t.Fatalf("a computed alt-screen must not be flagged, got %+v", r6.Check.Diagnostics)
	}
}

func TestTuiRunMirrorArms(t *testing.T) {
	rf := tuiRunMirror()

	// The flagged shape: an app config with no update and no service.
	r := vaultMirrorRegistry(t)
	out := rf([]native.Value{native.NewMap(native.NewOrderedMap())}, r)
	if len(out) != 1 || !out[0].Dynamic || !out[0].Parent.Equal(native.TAny) {
		t.Fatalf("run residual = %v, want one dynamic Any carrier", out)
	}
	if len(r.Check.Diagnostics) != 1 || r.Check.Diagnostics[0].Code != "tui_error" {
		t.Fatalf("a config-less run must flag tui_error, got %+v", r.Check.Diagnostics)
	}
	if want := "run: missing update"; r.Check.Diagnostics[0].Detail != want {
		t.Fatalf("detail = %q, want %q", r.Check.Diagnostics[0].Detail, want)
	}

	// A dynamic operand closes the gate: it matched optimistically, so the
	// run may not take the overload being modelled.
	r2 := vaultMirrorRegistry(t)
	dyn := native.NewMap(native.NewOrderedMap())
	dyn.Dynamic = true
	rf([]native.Value{dyn}, r2)
	if len(r2.Check.Diagnostics) != 0 {
		t.Fatalf("a dynamic app config must not be flagged, got %+v", r2.Check.Diagnostics)
	}

	// The wrong arity is dispatch's compile failure, not the mirror's — and
	// parseTuiApp would index out of range on an empty slice.
	r3 := vaultMirrorRegistry(t)
	rf(nil, r3)
	if len(r3.Check.Diagnostics) != 0 {
		t.Fatalf("a short arg slice must be left to dispatch, got %+v", r3.Check.Diagnostics)
	}
}

func TestTuiServeMirrorArms(t *testing.T) {
	rf := tuiServeMirror()
	emptyApp := native.NewMap(native.NewOrderedMap())

	// Transport options are validated FIRST, in the handler's order: a
	// port-less serve never reaches the app config.
	r := vaultMirrorRegistry(t)
	out := rf([]native.Value{native.NewMap(native.NewOrderedMap()), emptyApp}, r)
	if len(out) != 1 || !out[0].Dynamic || !out[0].Parent.Equal(native.TAny) {
		t.Fatalf("serve residual = %v, want one dynamic Any carrier", out)
	}
	if len(r.Check.Diagnostics) != 1 || r.Check.Diagnostics[0].Code != "tui_error" {
		t.Fatalf("a port-less serve must flag tui_error, got %+v", r.Check.Diagnostics)
	}
	if want := "serve: tcp: must be an Integer port"; r.Check.Diagnostics[0].Detail != want {
		t.Fatalf("detail = %q, want %q", r.Check.Diagnostics[0].Detail, want)
	}

	// With the transport valid, the app config is reached and its own
	// compile failure surfaces — the second half of the prefix.
	r2 := vaultMirrorRegistry(t)
	ok := tuiMirrorMap("tcp", native.NewInteger(0), "token", native.NewString("x"))
	rf([]native.Value{ok, emptyApp}, r2)
	if len(r2.Check.Diagnostics) != 1 || r2.Check.Diagnostics[0].Detail != "serve: missing update" {
		t.Fatalf("a valid transport must reach the app config, got %+v", r2.Check.Diagnostics)
	}

	// A computed transport option closes the gate for BOTH halves: if the
	// transport might fail at run time the app config is never reached, so
	// reporting its defect would name the wrong error.
	r3 := vaultMirrorRegistry(t)
	computed := tuiMirrorMap("tcp", core.NewCarrier(core.TInteger), "token", native.NewString("x"))
	rf([]native.Value{computed, emptyApp}, r3)
	if len(r3.Check.Diagnostics) != 0 {
		t.Fatalf("a computed port must not be flagged, got %+v", r3.Check.Diagnostics)
	}
}

// TestTuiMirrorsDeclineUnderPolicy covers the policy arms of all three
// mirrors: a denied word never reaches its validation at run time, so a
// usage diagnostic there would name the wrong error.
func TestTuiMirrorsDeclineUnderPolicy(t *testing.T) {
	pol, err := policy.Load("sandbox")
	if err != nil {
		t.Fatalf("load sandbox policy: %v", err)
	}
	emptyMap := native.NewMap(native.NewOrderedMap())
	cases := []struct {
		word string
		rf   native.ReturnsFunc
		args []native.Value
	}{
		{"open", tuiOpenMirror(), []native.Value{tuiMirrorMap("alt-screen", native.NewInteger(5))}},
		{"run", tuiRunMirror(), []native.Value{emptyMap}},
		// serve reaches its policy gates only with the transport VALID, so
		// this input is what exercises its declining arms.
		{"serve", tuiServeMirror(), []native.Value{
			tuiMirrorMap("tcp", native.NewInteger(0), "token", native.NewString("x")), emptyMap}},
	}
	for _, c := range cases {
		reg, rErr := native.DefaultRegistryWithPolicy(pol)
		if rErr != nil {
			t.Fatalf("%s: registry: %v", c.word, rErr)
		}
		done := reg.Check.Begin()
		c.rf(c.args, reg)
		if len(reg.Check.Diagnostics) != 0 {
			t.Fatalf("a policy-denied %s must not report a usage defect, got %+v",
				c.word, reg.Check.Diagnostics)
		}
		done()
	}
}

// TestTuiServeMirrorValidatesTransportBeforePolicy pins the ordering the
// handler defines. tuiServeHandler reads the transport options FIRST and
// only then consults a policy, so a malformed `tcp:` raises tui_error at
// run time even under a policy that denies the terminal outright. A
// mirror that consulted the terminal gate up front would drop that
// diagnostic — the bug this test exists to catch.
func TestTuiServeMirrorValidatesTransportBeforePolicy(t *testing.T) {
	pol, err := policy.Load("sandbox")
	if err != nil {
		t.Fatalf("load sandbox policy: %v", err)
	}
	reg, rErr := native.DefaultRegistryWithPolicy(pol)
	if rErr != nil {
		t.Fatalf("registry: %v", rErr)
	}
	t.Cleanup(reg.Check.Begin())

	// No tcp: at all — tuiServeOptsOf declines before any gate is reached.
	tuiServeMirror()([]native.Value{
		native.NewMap(native.NewOrderedMap()),
		native.NewMap(native.NewOrderedMap()),
	}, reg)
	if len(reg.Check.Diagnostics) != 1 || reg.Check.Diagnostics[0].Code != "tui_error" {
		t.Fatalf("a malformed transport must flag even under a denying policy, got %+v",
			reg.Check.Diagnostics)
	}
	if want := "serve: tcp: must be an Integer port"; reg.Check.Diagnostics[0].Detail != want {
		t.Fatalf("detail = %q, want %q", reg.Check.Diagnostics[0].Detail, want)
	}
}

// TestTuiServeMirrorDeclinesAtTerminalGate covers serve's SECOND gate.
// Under `sandbox` the net gate declines first, so the terminal gate is
// never reached with a denial; it takes the split profile
// (tui_serve_test.go's shape — network allowed, terminal not installed)
// to get past the net gate and be declined by the terminal one.
//
// The app config here is malformed, so the assertion is meaningful: the
// mirror stays silent because the run never reaches parseTuiApp.
func TestTuiServeMirrorDeclinesAtTerminalGate(t *testing.T) {
	split, err := policy.LoadInline(`{version: 1  name: "split"  scopes: {
	    network:  { words: { default: "allow" } }
	    terminal: { install: false }
	  }}`)
	if err != nil {
		t.Fatalf("load split policy: %v", err)
	}
	reg, rErr := native.DefaultRegistryWithPolicy(split)
	if rErr != nil {
		t.Fatalf("registry: %v", rErr)
	}
	t.Cleanup(reg.Check.Begin())

	tuiServeMirror()([]native.Value{
		tuiMirrorMap("tcp", native.NewInteger(0), "token", native.NewString("x")),
		native.NewMap(native.NewOrderedMap()), // missing update — never reached
	}, reg)
	if len(reg.Check.Diagnostics) != 0 {
		t.Fatalf("a terminal-denied serve must not report the app config, got %+v",
			reg.Check.Diagnostics)
	}
}
