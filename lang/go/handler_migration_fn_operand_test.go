package lang

import (
	"fmt"
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
// later re-dispatch), and their by-name Atom forms stay undeclared (the
// quoted class, a different worklist line).
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
			if sig.CompileEffect != core.CompileDefault {
				t.Errorf("%s by-name form %v: CompileEffect %v, want none (the quoted class is not this pilot's)", word, sig.ArgTypes(), sig.CompileEffect)
			}
		}
		if valueForms != 1 {
			t.Errorf("%s: %d value-form sigs, want 1", word, valueForms)
		}
	}
}

// TestModifierOverReturnedClosureRefusesWithParity pins the miscompile the
// pilot found and closed for the TYPED Function carrier: a capturing closure
// returned by a user fn, wrapped by a modifier word. Before the declaration
// was honoured, the gradual poly record lowered the wrap to
// OpCallNativePoly and the VM handed the native a ClosurePayload its
// FnDefInfo validation rejected — compiled `illegal_ref` against the
// interpreter's value. Now the strict slot REFUSES and the fallback agrees
// with the interpreter, value and taxonomy.
func TestModifierOverReturnedClosureRefusesWithParity(t *testing.T) {
	// Refusal+fallback-parity contract (the M2 tests' one-release hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	const mk = `def mk fn [[k:Integer][Function][([a:Integer b:Integer] => [(a sub b) add k])]]  `
	for _, c := range []struct{ name, src, want string }{
		{"usurp over a returned closure, def-bound", mk + `def r (usurp (mk 100))  r 10 3`, "[93]"},
		{"usurp over a returned closure, inline", mk + `usurp (mk 100) 10 3`, "[93]"},
		{"stack-args over a returned closure", mk + `def r (stack-args (mk 100))  10 3 r`, "[93]"},
		{"forward-args over a returned closure", mk + `def r (forward-args (mk 100))  r 10 3`, "[107]"},
		{"force-arity over a returned closure", mk + `def r (force-arity 2 (mk 100))  r 10 3`, "[107]"},
	} {
		fnValueM2Refusal(t, c.name, c.src, "unknown provenance")
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if compiled {
			t.Errorf("%s: ran compiled; the strict slot must refuse", c.name)
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if got := fmt.Sprint(gotI); got != c.want {
			t.Errorf("%s: interpreter %s, want %s", c.name, got, c.want)
		}
	}
	// The decline is narrow: the DYNAMIC Function carrier a sibling
	// modifier's gradual wrap produced (a composed chain over a map-read
	// native) keeps its poly record and compiles natively, as before.
	fnValueM2Native(t, "path-modifier.tsv:52 — usurp over a gradual forward-args wrapper",
		`def m {s:sub/v} end usurp (forward-args (m.s)) 10 3`, "[7]")
}
