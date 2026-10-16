package native

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"
)

// TestMembershipGoBoruParity is the convergence proof: a type defined in
// boru by a predicate body (`def Pos (Integer gt 10)`) and the SAME type
// defined in Go by a predicate func (MintMemberType) must answer `is`
// identically for every value — because, after the convergence, both
// route through the one shared membership contract (matchMembership /
// unifyMembership). If the two paths ever drift, a row here flips.
func TestMembershipGoBoruParity(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// boru path: install `Pos = Integer gt 10` by running the def.
	toks, perr := parser.Parse(`def Pos (Integer gt 10)`)
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	if _, rerr := NewTop(r).Run(toks); rerr != nil {
		t.Fatalf("install boru predicate type: %v", rerr)
	}
	posBoru := r.LookupTypeName("Pos")
	if posBoru == nil {
		t.Fatal("boru type Pos did not install")
	}

	// Go path: the same rule as a Go predicate, minted into the same
	// registry via the host helper.
	posGo := r.Types.MintMemberType("PosGo", core.TInteger, func(v Value) bool {
		n, err := v.AsConcreteInteger()
		return err == nil && n > 10
	})

	// Membership must agree across the whole range, including the wrong
	// family (a String is neither).
	values := []Value{
		NewInteger(-5), NewInteger(0), NewInteger(10), NewInteger(11),
		NewInteger(100), NewString("x"), NewBoolean(true),
	}
	for _, v := range values {
		gotBoru := v.Is(posBoru)
		gotGo := v.Is(posGo)
		if gotBoru != gotGo {
			t.Errorf("membership disagrees for %s: boru Pos=%v, Go PosGo=%v",
				v.String(), gotBoru, gotGo)
		}
	}

	// And the verdicts are the expected ones (not merely "equal but both
	// wrong"): only integers > 10 are members.
	if !addrOf(NewInteger(11)).Is(posGo) || addrOf(NewInteger(10)).Is(posGo) || addrOf(NewString("x")).Is(posGo) {
		t.Error("Go member type gave an unexpected verdict")
	}
	if !addrOf(NewInteger(11)).Is(posBoru) || addrOf(NewInteger(10)).Is(posBoru) || addrOf(NewString("x")).Is(posBoru) {
		t.Error("boru predicate type gave an unexpected verdict")
	}
}
