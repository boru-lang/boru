package native

import (
	"strings"
	"testing"
)

// Seam-7 coverage for native_flex.go (design/TEST-SEAMS.10.md). The
// deep-copy failure arms are reached with sealed Values whose Parent is
// mutated to a mismatched node family, so shape extraction (xmlParts /
// AsMap) fails while IsConcrete / IsFlexNode still hold.

// seam7BrokenXml is a concrete Value that claims Xml family but carries
// integer data, so FlexDeepCopy's xmlParts extraction fails.
func seam7BrokenXml() Value {
	v := NewInteger(5)
	v.SetParent(TXml)
	return v
}

func seam7FlexList(t *testing.T, r *Registry) Value {
	t.Helper()
	out, err := seam5Run(r, `flex [1 2 3]`)
	if err != nil || len(out) == 0 {
		t.Fatalf("build flex list: %v / %v", out, err)
	}
	return out[len(out)-1]
}

func seam7FlexXml(t *testing.T, r *Registry) Value {
	t.Helper()
	out, err := seam5Run(r, `flex <a>hi</a>`)
	if err != nil || len(out) == 0 {
		t.Fatalf("build flex xml: %v / %v", out, err)
	}
	return out[len(out)-1]
}

func TestSeam7FlexNodeReturnsGuards(t *testing.T) {
	r := seam5Reg(t)
	// The arity/nil-parent guard falls back to a dynamic Node carrier.
	if got := flexReturns(nil, r); len(got) != 1 || !got[0].Parent().ConformsTo(TNode) {
		t.Fatalf("flexReturns guard: %v", got)
	}
	if got := nodeReturns(nil, r); len(got) != 1 || !got[0].Parent().ConformsTo(TNode) {
		t.Fatalf("nodeReturns guard: %v", got)
	}
	// Positive family projections.
	om := NewOrderedMap()
	if got := flexReturns([]Value{NewMap(om)}, r); !got[0].Parent().ConformsTo(TFlexMap) {
		t.Fatalf("flexReturns map: %v", got[0].Parent())
	}
	if got := nodeReturns([]Value{NewMap(om)}, r); !got[0].Parent().ConformsTo(TMap) {
		t.Fatalf("nodeReturns map: %v", got[0].Parent())
	}
	// Unknown shape → dynamic Node.
	if got := flexReturns([]Value{NewInteger(1)}, r); !got[0].Parent().ConformsTo(TNode) {
		t.Fatalf("flexReturns default: %v", got[0].Parent())
	}
	if got := nodeReturns([]Value{NewInteger(1)}, r); !got[0].Parent().ConformsTo(TNode) {
		t.Fatalf("nodeReturns default: %v", got[0].Parent())
	}
}

func TestSeam7FlexHandler(t *testing.T) {
	r := seam5Reg(t)

	// Positive.
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	if _, err := flexHandler([]Value{NewMap(om)}, nil, nil, r); err != nil {
		t.Fatalf("flexHandler positive: %v", err)
	}
	// Not concrete.
	if _, err := flexHandler([]Value{NewTypeLiteral(TMap)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "concrete") {
		t.Fatalf("expected flex concrete error, got %v", err)
	}
	// FlexDeepCopy failure (broken Xml payload).
	if _, err := flexHandler([]Value{seam7BrokenXml()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "flex copy") {
		t.Fatalf("expected flex deep-copy error, got %v", err)
	}
}

func TestSeam7NodeHandler(t *testing.T) {
	r := seam5Reg(t)

	// Positive: node of a flex value converts back.
	if _, err := nodeHandler([]Value{seam7FlexList(t, r)}, nil, nil, r); err != nil {
		t.Fatalf("nodeHandler positive: %v", err)
	}
	// Not concrete.
	if _, err := nodeHandler([]Value{NewTypeLiteral(TList)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "concrete") {
		t.Fatalf("expected node concrete error, got %v", err)
	}
	// NOTE: nodeHandler's NodeDeepCopy-error arm (native_flex.go:159) is
	// provably unreachable. NodeDeepCopy short-circuits on
	// !containsFlex(v); containsFlex uses the identical Data-based
	// extraction (AsMap / AsList / xmlParts) the deep-copy then performs,
	// so any value that passes containsFlex cannot fail extraction. The
	// guard stays as defensive error propagation on an error-returning
	// call. (flexHandler differs: FlexDeepCopy has no containsFlex
	// pre-check, so its failure arm — line 148 — is reachable above.)
}

func TestSeam7AppendElemHandler(t *testing.T) {
	r := seam5Reg(t)
	fl := seam7FlexList(t, r)

	// Positive: append a single element.
	if _, err := appendElemHandler([]Value{NewInteger(9), fl}, nil, nil, r); err != nil {
		t.Fatalf("appendElemHandler positive: %v", err)
	}
	// Receiver not a FlexList.
	if _, err := appendElemHandler([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "FlexList") {
		t.Fatalf("expected FlexList receiver error, got %v", err)
	}
	// AdoptIntoFlex failure on the element (broken Xml).
	if _, err := appendElemHandler([]Value{seam7BrokenXml(), fl}, nil, nil, r); err == nil {
		t.Fatal("expected adopt error")
	}
}

func TestSeam7AppendListHandler(t *testing.T) {
	r := seam5Reg(t)
	fl := seam7FlexList(t, r)

	// Positive: concatenate a list's elements.
	if _, err := appendListHandler([]Value{NewList([]Value{NewInteger(1)}), fl}, nil, nil, r); err != nil {
		t.Fatalf("appendListHandler positive: %v", err)
	}
	// Receiver not a FlexList.
	if _, err := appendListHandler([]Value{NewList(nil), NewInteger(2)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "FlexList") {
		t.Fatalf("expected FlexList receiver error, got %v", err)
	}
	// Source not a concrete list.
	if _, err := appendListHandler([]Value{NewTypeLiteral(TList), fl}, nil, nil, r); err == nil {
		t.Fatal("expected concrete-list error")
	}
	// AdoptIntoFlex failure on an element.
	if _, err := appendListHandler([]Value{NewList([]Value{seam7BrokenXml()}), fl}, nil, nil, r); err == nil {
		t.Fatal("expected adopt error in list loop")
	}
}

func TestSeam7AppendXmlChildHandler(t *testing.T) {
	r := seam5Reg(t)
	fx := seam7FlexXml(t, r)

	// Positive: append a child.
	if _, err := appendXmlChildHandler([]Value{NewString("txt"), fx}, nil, nil, r); err != nil {
		t.Fatalf("appendXmlChildHandler positive: %v", err)
	}
	// Receiver not a FlexXml.
	if _, err := appendXmlChildHandler([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "FlexXml") {
		t.Fatalf("expected FlexXml receiver error, got %v", err)
	}
	// AdoptIntoFlex failure.
	if _, err := appendXmlChildHandler([]Value{seam7BrokenXml(), fx}, nil, nil, r); err == nil {
		t.Fatal("expected adopt error")
	}
}

func TestSeam7AppendXmlListHandler(t *testing.T) {
	r := seam5Reg(t)
	fx := seam7FlexXml(t, r)

	// Positive: splice a list of children.
	if _, err := appendXmlListHandler([]Value{NewList([]Value{NewString("t")}), fx}, nil, nil, r); err != nil {
		t.Fatalf("appendXmlListHandler positive: %v", err)
	}
	// Receiver not a FlexXml.
	if _, err := appendXmlListHandler([]Value{NewList(nil), NewInteger(2)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "FlexXml") {
		t.Fatalf("expected FlexXml receiver error, got %v", err)
	}
	// Source not a concrete list.
	if _, err := appendXmlListHandler([]Value{NewTypeLiteral(TList), fx}, nil, nil, r); err == nil {
		t.Fatal("expected concrete-list error")
	}
	// AdoptIntoFlex failure on an element.
	if _, err := appendXmlListHandler([]Value{NewList([]Value{seam7BrokenXml()}), fx}, nil, nil, r); err == nil {
		t.Fatal("expected adopt error in xml list loop")
	}
}
