package check

// Tests restored from the pre-carve tree (aa732c2).

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestUsurpCheckModeDiagnostics is restored from the pre-carve tree
// (aa732c2:eng/go/zz_cut_residual_test.go). It lives in check because the
// undefined_word half of the assertion comes from the check piece's
// CheckBraid.UndefinedWordCheckDiag registration (check_recovery.go); a
// core-only build runs inactiveUndefinedWordCheckDiag, which yields an
// empty-Code diagnostic.
func TestUsurpCheckModeDiagnostics(t *testing.T) {
	// In check mode both failures degrade to diagnostics + placeholder.
	r := covRegistry(t, nil)
	core.InstallDef(r, "plainv", core.NewInteger(1))
	done := r.Check.Begin()
	_, err := core.NewTop(r).Run([]core.Value{core.NewWordUsurp("no_such_word", false)})
	if err != nil {
		t.Errorf("check-mode usurp undefined errored hard: %v", err)
	}
	_, err = core.NewTop(r).Run([]core.Value{core.NewWordUsurp("plainv", false)})
	if err != nil {
		t.Errorf("check-mode usurp non-fn errored hard: %v", err)
	}
	var codes []string
	for _, d := range r.Check.Diagnostics {
		codes = append(codes, d.Code)
	}
	done()
	joined := strings.Join(codes, ",")
	if !strings.Contains(joined, "undefined_word") || !strings.Contains(joined, "illegal_ref") {
		t.Errorf("check-mode usurp diagnostics = %v", codes)
	}
}

// TestWordValCheckModeDiagnostics is restored from the pre-carve tree
// (aa732c2:eng/go/zz_cut_residual_test.go). It is the `/v` twin of
// TestUsurpCheckModeDiagnostics above and lives in check for the same
// reason: the undefined_word half of the assertion comes from the check
// piece's CheckBraid registration, so a core-only build would produce an
// empty-Code diagnostic instead.
//
// `/v` is TOTAL over binding kinds, so a non-fn binding is no longer a
// diagnostic — only an UNBOUND name is. The non-fn case stays here as
// the negative: it must produce NO diagnostic at all.
func TestWordValCheckModeDiagnostics(t *testing.T) {
	r := covRegistry(t, nil)
	core.InstallDef(r, "plainv", core.NewInteger(2))
	done := r.Check.Begin()
	if _, err := core.NewTop(r).Run([]core.Value{core.NewWordRef("no_such_word")}); err != nil {
		t.Errorf("check-mode /v undefined errored hard: %v", err)
	}
	if _, err := core.NewTop(r).Run([]core.Value{core.NewWordRef("plainv")}); err != nil {
		t.Errorf("check-mode /v non-fn errored hard: %v", err)
	}
	var codes []string
	for _, d := range r.Check.Diagnostics {
		codes = append(codes, d.Code)
	}
	done()
	joined := strings.Join(codes, ",")
	if !strings.Contains(joined, "undefined_word") {
		t.Errorf("check-mode /v diagnostics = %v, want an undefined_word", codes)
	}
	if strings.Contains(joined, "illegal_ref") {
		t.Errorf("check-mode /v diagnostics = %v, want NO illegal_ref for a non-fn binding", codes)
	}
}

func TestW8SpliceFnCheckTailCompaction(t *testing.T) {
	// A skipped marker between the args forces the compaction loop body.
	r := covRegistry(t, nil)
	e := core.NewTop(r)
	fnv := core.NewFunction(core.FnDefInfo{Signatures: []core.Signature{{
		Params: []core.FnParam{{Type: core.TInteger}, {Type: core.TInteger}},
	}}})
	e.Tape = core.NewTape([]core.Value{
		core.NewInteger(1), core.NewForward(core.ForwardInfo{}), core.NewInteger(2), fnv,
	}, core.StackHeadroom)
	e.Pointer = 3
	spliceFnCheckTail(e, 3, 2, []core.Value{core.NewCarrier(core.TAny)})
	// The two args and the fn-value literal are consumed; the skipped
	// marker is compacted down to the first arg slot and the result
	// carrier is spliced after it.
	if e.Pointer != 0 {
		t.Errorf("pointer = %d, want 0 (firstArgIdx)", e.Pointer)
	}
	if got := e.Tape.Len(); got != 2 {
		t.Fatalf("tape len = %d, want 2 (marker + carrier)", got)
	}
	if !core.IsForward(e.Tape.Ref(0)) {
		t.Errorf("tape[0] = %+v, want the Forward marker compacted down", e.Tape.At(0))
	}
	if got := e.Tape.At(1); !got.Carrier || !got.Parent.Equal(core.TAny) {
		t.Errorf("tape[1] = %+v, want the spliced Any carrier", got)
	}
}

// TestW8AnonFnCheckModeZeroArgActiveSlot is restored from the pre-carve tree
// (aa732c2:eng/go/zz_cut_residual_test.go). It lives in check because the
// behaviour under test is the check piece's spliceAnonCheckResult
// (check_recovery.go), installed into core.CheckBraid.SpliceAnonCheckResult
// by this module's init. The core-side twin
// (core/go/engine_seam8_test.go: TestW8AnonFnCheckModeZeroArg) runs the same
// path against the INACTIVE braid default, so it does not cover the real
// implementation.
func TestW8AnonFnCheckModeZeroArgActiveSlot(t *testing.T) {
	// A 0-arg anonymous fn dispatched in check mode routes through
	// ExecFnDefSigStackMatch's checkMode branch → spliceAnonCheckResult(0).
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	fnDef := zzAnonDef(nil, []*core.Type{core.TInteger}, parenBody(core.NewInteger(7)))
	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewFunction(fnDef)}, core.StackHeadroom)
	e.Pointer = 0
	if err := e.ExecFnDefSigStackMatch(0, fnDef, nil); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}

func TestW8AnonFnCheckModeEmptyBodyActiveSlot(t *testing.T) {
	// A body that yields no value → spliceAnonCheckResult's empty-result
	// default (NewCarrier(TAny)).
	//
	// The core-side twin (TestW8ExecFnDefStackMatchCheckZeroArgEmptyBody)
	// runs against the INACTIVE CheckBraid default; here the real check
	// implementation is installed, so this same direct
	// ExecFnDefSigStackMatch call covers spliceAnonCheckResult itself —
	// specifically the nArgs == 0 tail arm of spliceFnCheckTail.
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	fnDef := zzAnonDef(nil, nil, parenBody())
	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewFunction(fnDef)}, core.StackHeadroom)
	e.Pointer = 0
	if err := e.ExecFnDefSigStackMatch(0, fnDef, nil); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}

// TestW8AnonFnCheckModeNamedParamsStackActiveSlot is restored from the
// pre-carve tree (aa732c2:eng/go/zz_cut_residual_test.go). It lives in check
// because the behaviour under test is the check piece's
// spliceAnonCheckResult (check_recovery.go), installed into
// core.CheckBraid.SpliceAnonCheckResult by this module's init. The core-side
// twin (core/go/engine_seam8_test.go: TestW8ExecFnDefStackMatchCheckNamed)
// makes the identical call but runs against the INACTIVE braid default, so it
// does not reach the real implementation. Its unnamed-param counterpart,
// TestW8AnonFnCheckModeUnnamedParamsStackActiveSlot, already survives in
// check/go/zz_cut_residual_test.go; this is the hasNamed arm.
func TestW8AnonFnCheckModeNamedParamsStackActiveSlot(t *testing.T) {
	// A named-param anonymous fn with stack args in check mode →
	// spliceAnonCheckResult(nArgs) via the hasNamed branch.
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()
	fnDef := zzAnonDef(
		[]core.FnParam{{Name: "a", Type: core.TInteger}, {Name: "b", Type: core.TInteger}},
		[]*core.Type{core.TInteger},
		parenBody(core.NewWord("cadd"), core.NewWord("a"), core.NewWord("b")),
	)
	e := core.NewTop(r)
	e.Tape = core.NewTape(
		[]core.Value{core.NewInteger(2), core.NewInteger(3), core.NewFunction(fnDef)},
		core.StackHeadroom,
	)
	e.Pointer = 2
	if err := e.ExecFnDefSigStackMatch(2, fnDef, []core.Value{core.NewInteger(2), core.NewInteger(3)}); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
}

func TestOrderingReturnsFn(t *testing.T) {
	r := newTestRegistry(t)
	fn := OrderingReturnsFn(core.LtHandler, core.TBoolean)

	// Cross-family determinate operands: diagnostic recorded.
	before := len(r.Check.Diagnostics)
	out := fn([]core.Value{core.NewString("a"), core.NewInteger(1)}, r)
	if len(out) != 1 || !out[0].Carrier || !out[0].Parent.Equal(core.TBoolean) {
		t.Fatalf("OrderingReturnsFn result = %v, want Boolean carrier", out)
	}
	if len(r.Check.Diagnostics) != before+1 {
		t.Errorf("cross-family static pair added %d diagnostics, want 1",
			len(r.Check.Diagnostics)-before)
	}
	if r.Check.Diagnostics[len(r.Check.Diagnostics)-1].Code != "incomparable" {
		t.Errorf("diagnostic code = %q", r.Check.Diagnostics[len(r.Check.Diagnostics)-1].Code)
	}

	// A dynamic operand is not determinate — no diagnostic.
	before = len(r.Check.Diagnostics)
	dyn := core.NewCarrier(core.TAny)
	dyn.Dynamic = true
	fn([]core.Value{dyn, core.NewInteger(1)}, r)
	if len(r.Check.Diagnostics) != before {
		t.Error("dynamic operand produced a static incomparable diagnostic")
	}

	// Same-family determinate operands: no diagnostic.
	before = len(r.Check.Diagnostics)
	fn([]core.Value{core.NewInteger(1), core.NewInteger(2)}, r)
	if len(r.Check.Diagnostics) != before {
		t.Error("orderable pair produced a diagnostic")
	}
}

// TestRunCarrierBodyAndStacksEqual is restored from the pre-carve tree
// (aa732c2:eng/go/zz_cut_residual_test.go). It lives in check because it
// touches the unexported carrierStacksEqual, and core.RunCarrierBody is defined
// here too (eng only re-exports it through the generated facade).
func TestRunCarrierBodyAndStacksEqual(t *testing.T) {
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	defer done()

	stk := core.RunCarrierBody(r, core.NewList([]core.Value{
		core.NewWord("cadd"), core.NewInteger(1), core.NewInteger(2),
	}))
	if len(stk) != 1 || !stk[0].Parent.ConformsTo(core.TInteger) {
		t.Errorf("core.RunCarrierBody residual = %v", stk)
	}
	// A non-concrete body yields nil.
	if stk := core.RunCarrierBody(r, core.NewTypeLiteral(core.TList)); stk != nil {
		t.Errorf("type-literal body residual = %v", stk)
	}

	// carrierStacksEqual: equal values equal, different values not,
	// mismatched lengths never.
	a := []core.Value{core.NewInteger(1)}
	b := []core.Value{core.NewInteger(1)}
	c := []core.Value{core.NewString("x")}
	if !carrierStacksEqual(a, b) {
		t.Error("equal stacks unequal")
	}
	if carrierStacksEqual(a, c) {
		t.Error("different stacks equal")
	}
	if carrierStacksEqual(a, nil) {
		t.Error("length mismatch equal")
	}
}
