package native

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The fn-predicate bind's DECLINED-record compile failure (the concrete-permitting
// twin of recordTypedBindOrDecline): with an ACTIVE recorder and a body whose
// operand has no resolvable provenance, RecordTypedBind declines and the
// decline closure marks the program uncompilable — never the silent bake the
// 2026-07-15 flip attempt caught.
func TestRecordTypedBindOrFailToCompileConcreteDecline(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Check.Begin()()
	defer r.Check.BeginCompilePass()()
	dyn := core.NewCarrier(TAny)
	dyn.Dynamic = true
	dyn.ID = ""
	cons := core.NewCarrier(TAny)
	out := recordTypedBindOrDeclineConcrete(r, func() core.TypedBindSpec {
		return core.TypedBindSpec{Kind: core.TypedBindPredicate, Name: "zz", Describe: "Zz", Cons: &cons}
	}, dyn, dyn, core.SrcPos{}, func() { markFnPredicateBindUncompilable(r, "zz") })
	if !out.Dynamic {
		t.Fatal("the declined record must return the bound value unchanged")
	}
	_, reason, _ := r.Check.Recorder().(*compiler.EmitState).Finalize(nil)
	if !strings.Contains(reason, "fn-predicate bind is runtime-evaluated") {
		t.Fatalf("declined record must decline; Finalize reason = %q", reason)
	}
}

// miniHookToksMaterialisable's decline arms: a carrier/dynamic token and a
// non-inert marker token (a Forward) both divert the compile pass onto the
// standard transducer call.
func TestMiniHookToksMaterialisableArms(t *testing.T) {
	if !miniHookToksMaterialisable([]core.Value{NewString("x"), NewWord("w"), NewEnd()}) {
		t.Fatal("inert tokens must be materialisable")
	}
	if miniHookToksMaterialisable([]core.Value{NewCarrier(TAny)}) {
		t.Fatal("a carrier token must not be materialisable")
	}
	dyn := NewCarrier(TAny)
	dyn.Dynamic = true
	if miniHookToksMaterialisable([]core.Value{dyn}) {
		t.Fatal("a dynamic token must not be materialisable")
	}
	if miniHookToksMaterialisable([]core.Value{core.NewExtension(TAny, 1)}) {
		t.Fatal("an extension-payload token must not be materialisable")
	}
}
