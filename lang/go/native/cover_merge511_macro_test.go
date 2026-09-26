package native

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// cover_merge511_macro_test.go pins the macro fn-dispatch arms (main's #510)
// the corpus never reaches — the merged ADR-008 gate on the reverse-order NUR
// run's merge of main's #511 — each with the verdict the arm exists to give.

// recordMacroFnDispatch: no dispatch native installed (the module not
// imported) and no signature of the call's arity both leave the caller on its
// degrade; a call with no word position anchors at its last operand.
func TestRecordMacroFnDispatchArms(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Check.Begin()()
	defer r.Check.BeginCompilePass()()
	lead := NewString("zz")
	src := NewString("ab")
	if _, ok := recordMacroFnDispatch(r, capMiniLangFnDispatch, "minilang-fn-dispatch", []Value{lead, src}); ok {
		t.Fatal("with no dispatch native installed nothing records")
	}
	three := Signature{Args: []*Type{TAny, TString, TMap}, BarrierPos: -1, Impl: core.Go(nil)}
	InstallMiniLangFnDispatch(r, &FnDefInfo{Name: "minilang-fn-dispatch", Signatures: []Signature{three}})
	if _, ok := recordMacroFnDispatch(r, capMiniLangFnDispatch, "minilang-fn-dispatch", []Value{lead, src}); ok {
		t.Fatal("a call of an arity the native does not take records nothing")
	}
	r.Check.CurWordPos = core.SrcPos{}
	opts := core.WithPosAt(NewMap(NewOrderedMap()), core.SrcPos{Row: 3, Col: 9})
	if out, ok := recordMacroFnDispatch(r, capMiniLangFnDispatch, "minilang-fn-dispatch", []Value{lead, src, opts}); !ok || !out.Parent.ConformsTo(TAny) {
		t.Fatalf("the native's own arity records, anchored at the last operand: %v %v", out, ok)
	}
}

// The install entry points ignore a missing registry or native, and the
// fn-value filter partial is built over the value's own signatures.
func TestMacroDispatchInstallGuardsAndPartial(t *testing.T) {
	InstallParseLangLeadDispatch(nil, nil)
	InstallEmitLangFnDispatch(nil, nil)
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	InstallParseLangLeadDispatch(r, nil)
	InstallMiniLangFnDispatch(r, nil)
	if _, ok, _ := core.Cap[*FnDefInfo](r, capMiniLangFnDispatch); ok {
		t.Error("a nil native installs nothing")
	}
	fd := FnDefInfo{Signatures: []Signature{{
		Params: []FnParam{{Name: "src", Type: TString}, {Name: "opts", Type: TMap}, {Name: "x", Type: TAny}},
		Impl:   core.Boru([]Value{NewWord("x")}),
	}}}
	p := MiniPartialFromFn(fd, []Value{NewString("ab"), NewMap(NewOrderedMap()), NewEnd()})
	if !p.Parent.ConformsTo(TFunction) {
		t.Errorf("the filter partial is a fn value, got %v", p)
	}
}
