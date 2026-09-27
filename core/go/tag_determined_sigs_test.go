package core

import "testing"

// A signature set is tag-determined only when nothing but the operands'
// construction tags can decide its first match.
func TestTagDeterminedSigs(t *testing.T) {
	plain := []Signature{{Args: []*Type{TInteger, TInteger}}, {Args: []*Type{TString, TString}}}
	if !TagDeterminedSigs(plain, 2) {
		t.Error("builtin tag-matched args are tag-determined")
	}
	pat := NewInteger(1)
	withPattern := []Signature{{Params: []FnParam{{Type: TInteger, Pattern: &pat}}}}
	typeArg := []Signature{{Args: []*Type{TInteger}, TypeArgs: map[int]bool{0: true}}}
	options := []Signature{{Args: []*Type{TOptions}}}
	user := &Type{Origin: OriginUserDef}
	userArg := []Signature{{Args: []*Type{user}}}
	nilArg := []Signature{{Args: []*Type{nil}}}
	content := &Type{Origin: OriginBuiltin, tmeta: &typeMeta{Behavior: typeMembershipBehavior{}}}
	contentArg := []Signature{{Args: []*Type{content}}}
	for name, sigs := range map[string][]Signature{
		"pattern": withPattern, "type-literal slot": typeArg, "Options": options,
		"user type": userArg, "nil type": nilArg, "content membership": contentArg,
	} {
		if TagDeterminedSigs(sigs, 1) {
			t.Errorf("%s: must not be tag-determined", name)
		}
	}
	// Signatures of another arity do not take part.
	if !TagDeterminedSigs(append(withPattern, plain...), 2) {
		t.Error("a 1-arg pattern sig does not affect a 2-arg match")
	}
}

// Only a concrete, unascribed, non-carrier value is keyed by its tag.
func TestTagKeyable(t *testing.T) {
	if !TagKeyable(NewInteger(1)) {
		t.Error("a concrete integer is tag-keyable")
	}
	carrier := NewCarrier(TInteger)
	dyn := NewDynamicCarrier(TInteger)
	asc := NewInteger(1)
	asc.SetAscribed(TNumber)
	for name, v := range map[string]Value{
		"carrier": carrier, "dynamic": dyn, "bare type": NewTypeLiteral(TInteger), "ascribed": asc, "no parent": {},
	} {
		if TagKeyable(v) {
			t.Errorf("%s: must not be tag-keyable", name)
		}
	}
}
