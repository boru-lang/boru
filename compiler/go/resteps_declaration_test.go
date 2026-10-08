package compiler

// CompileResteps is the DECLARED form of the refusal the recorder used to
// make on the zero value's silence: a word whose handler's result is
// re-stepped by the engine (the by-name modifier words, valof, the
// mini/parse/emit splices, apply) cannot bake as a CALL_NATIVE. S2a of
// design/FULL-COMPILATION-REPLAN.0.md gave the flag to those words; these
// pins hold what the gates owe it — the refusal is read FIRST, before and
// regardless of any admission the same signature carries, and the reason
// names the declaration. Every declarer today also runs in check mode, so
// the RunInCheckMode screen reaches the gates first for them; the hand-built
// sigs here carry no RunInCheck precisely so the flag's own arm is the one
// under test, as it would be for a future declarer that did not.

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func noopHandler(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
	return nil, nil
}

// restepsRegistry registers a quoted-operand word declaring CompileResteps
// BESIDE an admission (CompileQuoteKey | CompileQuoteInert), so each gate's
// test shows the refusal outranks the admission rather than merely that an
// undeclared sig declines.
func restepsRegistry(t *testing.T) (*core.Registry, *core.Signature) {
	t.Helper()
	r := newTestRegistry(t)
	armEmit(r)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "zz-resteps",
		Signatures: []core.Signature{{
			Args:          []*core.Type{core.TAtom},
			QuoteArgs:     map[int]bool{0: true},
			Impl:          core.Go(noopHandler),
			Returns:       []*core.Type{},
			BarrierPos:    -1,
			CompileEffect: core.CompileResteps | core.CompileQuoteKey | core.CompileQuoteInert,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	fd := r.Lookup("zz-resteps")
	if fd == nil || len(fd.Signatures) == 0 {
		t.Fatal("registration not found")
	}
	return r, &fd.Signatures[0]
}

func TestRestepsSigReadsTheDeclaration(t *testing.T) {
	_, sig := restepsRegistry(t)
	if !restepsSig(sig) {
		t.Error("a declaring sig must be recognised")
	}
	if restepsSig(nil) {
		t.Error("nil sig must decline")
	}
	plain := *sig
	plain.CompileEffect = core.CompileQuoteKey
	if restepsSig(&plain) {
		t.Error("an admission alone is not the refusal")
	}
}

// TestRestepsDeclinesQuotedOperandGate: the uncompilable switch's quoted-
// operand arm declines a CompileResteps declarer although it also carries
// CompileQuoteKey (which admits set/del there) and the caller has proven
// quoteInertOK; the reason names the declaration and keeps the bucket
// prefix the coverage histogram reads.
func TestRestepsDeclinesQuotedOperandGate(t *testing.T) {
	_, sig := restepsRegistry(t)
	es := NewEmitState()
	args := []core.Value{core.NewAtom("k")}
	if !es.recordCallCompileFailure("zz-resteps", sig, args, nil, core.SrcPos{}, false, true) || es.Compilable {
		t.Fatal("a CompileResteps declarer must decline at the quoted-operand arm")
	}
	if !strings.HasPrefix(es.Reason, "quoted-operand word zz-resteps") || !strings.Contains(es.Reason, "CompileResteps") {
		t.Errorf("reason = %q, want the quoted-operand prefix naming CompileResteps", es.Reason)
	}
	// The same shape WITHOUT the refusal rides the key admission: that is the
	// arm the declaration outranks.
	admitted := *sig
	admitted.CompileEffect = core.CompileQuoteKey
	es = NewEmitState()
	if es.recordCallCompileFailure("zz-resteps", &admitted, args, nil, core.SrcPos{}, false, false) {
		t.Fatal("the key admission alone must fall through to an ordinary record")
	}
}

// TestRestepsDeclinesInertExemption: quoteOperandInertOK reads the refusal
// before its CompileQuoteInert branch, so an inert atom operand buys nothing.
func TestRestepsDeclinesInertExemption(t *testing.T) {
	r, sig := restepsRegistry(t)
	inert := []core.Value{core.NewAtom("k")}
	if quoteOperandInertOK(r, "zz-resteps", sig, inert) {
		t.Error("a CompileResteps declarer must not ride the inert-const exemption")
	}
	admitted := *sig
	admitted.CompileEffect = core.CompileQuoteInert
	if !quoteOperandInertOK(r, "zz-resteps", &admitted, inert) {
		t.Error("the inert admission alone must still be admitted — the premise of the negative above")
	}
}

// TestRestepsDeclinesPolyRecord: recordPolyCall's quoted line declines the
// declarer although CompileQuoteKey would admit it (the twin of
// TestS6aTryRecordPolyQuotedDeclarationAdmits).
func TestRestepsDeclinesPolyRecord(t *testing.T) {
	r, sig := restepsRegistry(t)
	if tryRecordPoly(r, "zz-resteps", sig, []core.Value{core.NewAtom("k")}, []core.Value{}, core.SrcPos{}, true, nil, false, nil) {
		t.Error("a CompileResteps declarer must not poly, whatever admission it also carries")
	}
}

// TestRestepsDeclinesFnOperandSlot: RecordCallOperands declines a Function-
// typed slot on a CompileResteps declarer even when CompileReadsFn would
// otherwise let the fn ride as an inert const (apply's shape, had it been
// given the permissive flag), and says why.
func TestRestepsDeclinesFnOperandSlot(t *testing.T) {
	es := NewEmitState()
	sig := &core.Signature{
		Args:          []*core.Type{core.TFunction},
		CompileEffect: core.CompileResteps | core.CompileReadsFn,
	}
	if _, ok := es.RecordCallOperands("zz-apply", sig, []core.Value{core.NewDynamicCarrier(core.TFunction)}); ok || es.Compilable {
		t.Fatal("a CompileResteps declarer's fn slot must decline")
	}
	if !strings.HasPrefix(es.Reason, "function-valued operand at zz-apply") || !strings.Contains(es.Reason, "CompileResteps") {
		t.Errorf("reason = %q, want the fn-operand prefix naming CompileResteps", es.Reason)
	}
	// Without the refusal, CompileReadsFn admits the slot: the premise. (The
	// bare carrier then declines later, on provenance — what matters here is
	// that the fn-slot arm let it through.)
	es = NewEmitState()
	reads := &core.Signature{Args: []*core.Type{core.TFunction}, CompileEffect: core.CompileReadsFn}
	es.RecordCallOperands("zz-apply", reads, []core.Value{core.NewDynamicCarrier(core.TFunction)})
	if strings.HasPrefix(es.Reason, "function-valued operand") {
		t.Errorf("CompileReadsFn alone must admit the fn slot (reason %q)", es.Reason)
	}
	// An undeclared fn slot keeps the plain Stage-3 reason.
	es = NewEmitState()
	if _, ok := es.RecordCallOperands("zz-apply", &core.Signature{Args: []*core.Type{core.TFunction}}, []core.Value{core.NewDynamicCarrier(core.TFunction)}); ok || es.Compilable {
		t.Fatal("an undeclared fn slot must decline")
	}
	if es.Reason != "function-valued operand at zz-apply (Stage 3)" {
		t.Errorf("undeclared reason = %q", es.Reason)
	}
}
