package lang

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	native "github.com/boru-lang/boru/lang/go/native"
)

// handler_migration_fn_operand_test.go pins the fn-operand pilot of the
// handler-migration line (design/HANDLER-MIGRATION-LINE.0.md, Progress
// 2026-09-18): the dispatch-modifier words' VALUE forms declare their
// handler contract, and the declaration is honoured at the one recorder
// seat those check-mode words reach — the gradual poly record.

// TestModifierValueFormsDeclareStoreFnStrict is the declaration census's
// per-word pin: usurp / stack-args / forward-args [Function] and
// force-arity [Integer Function] carry CompileStoresFn|CompileFnHandlerStrict
// (the fn is validated as an FnDefInfo and stored in the wrapper for the
// later re-dispatch), and their by-name Atom forms declare CompileResteps
// (the quoted class, S2a of design/FULL-COMPILATION-REPLAN.0.md: the
// quoted name resolves to a wrapper the tape re-steps — a declared
// refusal, never an inert or key admission).
func TestModifierValueFormsDeclareStoreFnStrict(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	want := core.CompileStoresFn | core.CompileFnHandlerStrict
	for _, word := range []string{"usurp", "stack-args", "forward-args", "force-arity"} {
		fd := reg.Lookup(word)
		if fd == nil {
			t.Fatalf("%s: not registered", word)
		}
		var valueForms int
		for i := range fd.Signatures {
			sig := &fd.Signatures[i]
			fnSlot := false
			for _, ty := range sig.ArgTypes() {
				if ty != nil && ty.ConformsTo(core.TFunction) {
					fnSlot = true
				}
			}
			if fnSlot {
				valueForms++
				if !sig.CompileEffect.Has(core.CompileStoresFn) || !sig.CompileEffect.Has(core.CompileFnHandlerStrict) {
					t.Errorf("%s value form %v: CompileEffect %v, want CompileStoresFn|CompileFnHandlerStrict (%v)", word, sig.ArgTypes(), sig.CompileEffect, want)
				}
				continue
			}
			if sig.CompileEffect != core.CompileResteps {
				t.Errorf("%s by-name form %v: CompileEffect %v, want exactly CompileResteps (the re-stepped wrapper, declared as the refusal it is)", word, sig.ArgTypes(), sig.CompileEffect)
			}
		}
		if valueForms != 1 {
			t.Errorf("%s: %d value-form sigs, want 1", word, valueForms)
		}
	}
}
