package core

import (
	"strings"
	"testing"
)

// The sugar-role compile failure contract (ADR-012 amendment): a registry with
// no binding for a stepped role fails LOUDLY — the bare kernel (calc)
// runs with zero bindings, so every marker kind must decline cleanly
// rather than lower to an invented name. These tests pin every
// unbound-role error arm and the role-table edge cases the language
// layer never exercises (it always binds).

func TestSugarRoleTableEdges(t *testing.T) {
	// nil-receiver safety: the helpers are nil-safe like the rest of
	// the Registry surface.
	var nilR *Registry
	nilR.BindSugarWord(SugarLambda, "afn") // must not panic
	if name, ok := nilR.SugarWord(SugarLambda); ok || name != "" {
		t.Errorf("nil registry must bind nothing, got %q/%v", name, ok)
	}
	// A fresh registry has no role table until the first bind.
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := r.SugarWord(SugarLambda); ok || name != "" {
		t.Errorf("unbound role must miss, got %q/%v", name, ok)
	}
	r.BindSugarWord(SugarLambda, "afn")
	if name, ok := r.SugarWord(SugarLambda); !ok || name != "afn" {
		t.Errorf("bound role must resolve, got %q/%v", name, ok)
	}
}

func TestSugarExpansionUnboundRoles(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Source = "test-src"
	cases := []struct {
		name string
		info SugarInfo
		head bool
	}{
		{"usurp", SugarInfo{Kind: SugarUsurp}, false},
		{"force-arity", SugarInfo{Kind: SugarForceArity, N: 2}, false},
		{"mini", SugarInfo{Kind: SugarMini, Name: "path", Src: "a/b"}, false},
		{"type-bound", SugarInfo{Kind: SugarTypeBound, Items: []Value{NewList([]Value{NewAtom("X")})}}, false},
		{"angle-head", SugarInfo{Kind: SugarAngle, Name: "Box", Head: NewList(nil)}, true},
		{"angle-use", SugarInfo{Kind: SugarAngle, Name: "Box", Items: []Value{NewWord("Integer")}}, false},
	}
	for _, c := range cases {
		src := NewSugar(c.info)
		if _, serr := SugarExpansion(r, c.info, src, c.head); serr == nil {
			t.Errorf("%s: unbound role must decline", c.name)
		} else if !strings.Contains(serr.Error(), "sugar_unbound") {
			t.Errorf("%s: want sugar_unbound, got %v", c.name, serr)
		}
	}
	// The nil-registry variant of the unbound error (source falls to "").
	var nilR *Registry
	if _, serr := SugarExpansion(nilR, SugarInfo{Kind: SugarUsurp}, NewSugar(SugarInfo{Kind: SugarUsurp}), false); serr == nil ||
		!strings.Contains(serr.Error(), "sugar_unbound") {
		t.Errorf("nil registry: want sugar_unbound, got %v", serr)
	}
	// An unknown kind declines on both registry shapes.
	bogus := SugarInfo{Kind: SugarKind("bogus")}
	if _, serr := SugarExpansion(r, bogus, NewSugar(bogus), false); serr == nil ||
		!strings.Contains(serr.Error(), "unknown sugar kind") {
		t.Errorf("unknown kind: want compile failure, got %v", serr)
	}
	if _, serr := SugarExpansion(nilR, bogus, NewSugar(bogus), false); serr == nil ||
		!strings.Contains(serr.Error(), "unknown sugar kind") {
		t.Errorf("unknown kind (nil registry): want compile failure, got %v", serr)
	}
}

// A marker STEPPED on a bindingless registry surfaces sugar_unbound
// from the step loop (stepSugar's error path), and a marker met by a
// dispatching word's forward scan is a plain boundary when its role is
// unbound — the dispatch then fails on its own terms instead of
// swallowing the marker (resolveForwardArgs' expandScanSugar use-site
// failure + matchSignature's scan failure arms).
func TestSugarUnboundAtStepAndScan(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := New(r)
	_, rerr := e.Run([]Value{NewSugar(SugarInfo{Kind: SugarUsurp})})
	if rerr == nil || !strings.Contains(rerr.Error(), "sugar_unbound") {
		t.Errorf("stepped unbound marker: want sugar_unbound, got %v", rerr)
	}

	// A native whose forward window meets the unbound marker: the scan
	// treats it as a boundary, the dispatch fails loudly (insufficient
	// args), and the marker is NOT silently consumed.
	r2, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r2.RegisterNativeFunc(NativeFunc{
		Name: "sink",
		Signatures: []Signature{{
			Args: []*Type{TAny},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			}), BarrierPos: -1,
		}},
	})
	if r2.Err() != nil {
		t.Fatal(r2.Err())
	}
	e2 := New(r2)
	_, rerr2 := e2.Run([]Value{NewWord("sink"), NewSugar(SugarInfo{Kind: SugarUsurp})})
	if rerr2 == nil {
		t.Errorf("unbound marker in a forward window must not dispatch silently")
	}
}

// kernelFormatDefault renders every marker kind distinctly (the
// Behavior-format twin of Value.String's sugar arm).
func TestKernelFormatSugarMarkers(t *testing.T) {
	cases := []struct {
		info SugarInfo
		want string
	}{
		{SugarInfo{Kind: SugarForceArity, N: 3}, "sugar(force-arity 3)"},
		{SugarInfo{Kind: SugarMini, Name: "path", Src: "a/b"}, "sugar(mini path 'a/b')"},
		{SugarInfo{Kind: SugarLambda}, "sugar(lambda)"},
	}
	for _, c := range cases {
		if got := kernelFormatDefault(NewSugar(c.info)); got != c.want {
			t.Errorf("format %v: got %q want %q", c.info.Kind, got, c.want)
		}
	}
}

// A sugar marker as an inert-const MEMBER (macro output riding a baked
// compound) is admitted only when the value streams it carries are
// themselves inert — a carrier in the Head or Items keeps the compound
// off the const pool.
func TestInertConstMemberSugar(t *testing.T) {
	inert := NewSugar(SugarInfo{
		Kind: SugarAngle, Name: "Box",
		Head:  NewList([]Value{NewWord("T")}),
		Items: []Value{NewWord("Integer")},
	})
	if !IsInertConstMember(inert) {
		t.Errorf("an inert angle marker must bake as a const member")
	}
	carrierHead := NewSugar(SugarInfo{Kind: SugarAngle, Name: "Box", Head: NewCarrier(TList)})
	if IsInertConstMember(carrierHead) {
		t.Errorf("a carrier Head must keep the marker off the const pool")
	}
	carrierItem := NewSugar(SugarInfo{Kind: SugarAngle, Name: "Box", Items: []Value{NewCarrier(TList)}})
	if IsInertConstMember(carrierItem) {
		t.Errorf("a carrier Item must keep the marker off the const pool")
	}
}
