package core

// Seam-7 (cluster A4): in-package unit tests for previously-unreached
// blocks in value.go — nil-receiver guards, accessor type-mismatch error
// arms, nil-arg constructor defaults, and kernelFormatDefault's marker /
// instance arms. Driven by direct calls with crafted Values. Per
// design/TEST-SEAMS.10.md.

import (
	"testing"

	"github.com/cockroachdb/apd/v3"
)

// --- FnDefInfo nil receivers ----------------------------------------------

func TestS7FnDefInfoNilReceivers(t *testing.T) {
	var fn *FnDefInfo
	if fn.HasForwardSigs() {
		t.Error("nil HasForwardSigs must be false")
	}
	if fn.OwnSigs() != nil {
		t.Error("nil OwnSigs must be nil")
	}
	if _, ok := fn.FirstOwnSig(); ok {
		t.Error("nil FirstOwnSig must be false")
	}
}

// --- Big formatters -------------------------------------------------------

func TestS7BigFormattersEdges(t *testing.T) {
	if got := FormatBigInteger(nil); got != "0d0" {
		t.Errorf("FormatBigInteger(nil) = %q", got)
	}
	if got := FormatBigDecimal(nil); got != "0d0" {
		t.Errorf("FormatBigDecimal(nil) = %q", got)
	}
	neg := apd.New(-5, 0)
	if got := FormatBigDecimal(neg); got != "-0d5" {
		t.Errorf("FormatBigDecimal(-5) = %q, want -0d5", got)
	}
}

// --- constructor nil-arg defaults & atom referent -------------------------

func TestS7ConstructorNilDefaults(t *testing.T) {
	if got := NewTypeLiteral(nil); ParentOf(&got) != nil || DataOf(&got) != nil {
		t.Errorf("NewTypeLiteral(nil) = %+v, want zero", got)
	}
	if s := NewStore(nil); ParentOf(&s) == nil {
		t.Error("NewStore(nil) must default its type")
	}
	if s := NewStoreWithPrototype(nil, nil); ParentOf(&s) == nil {
		t.Error("NewStoreWithPrototype(nil,...) must default its type")
	}
	// SetAtomReferent on a non-atom leaves it as it was.
	got, two := NewInteger(1), NewInteger(2)
	before := got.occ
	SetAtomReferent(&got, &two)
	if n, _ := AsInteger(got); n != 1 || got.occ != before {
		t.Errorf("SetAtomReferent(non-atom) = %v, want unchanged 1", got)
	}
}

// --- accessor type-mismatch error arms ------------------------------------

func TestS7AccessorErrorArms(t *testing.T) {
	notIt := NewInteger(1)
	if _, err := AsParenExpr(&notIt); err == nil {
		t.Error("AsParenExpr(int) must error")
	}
	if _, err := AsInterpString(notIt); err == nil {
		t.Error("AsInterpString(int) must error")
	}
	if _, err := AsReturnCheck(&notIt); err == nil {
		t.Error("AsReturnCheck(int) must error")
	}
	if _, err := AsDefCleanup(&notIt); err == nil {
		t.Error("AsDefCleanup(int) must error")
	}
	if _, err := AsNegation(notIt); err == nil {
		t.Error("AsNegation(int) must error")
	}
	if _, err := AsForward(&notIt); err == nil {
		t.Error("AsForward(int) must error")
	}
	if _, err := AsFlexXml(notIt); err == nil {
		t.Error("AsFlexXml(int) must error")
	}
	if _, err := AsMutableList(NewTypeLiteral(TList)); err == nil {
		t.Error("AsMutableList(type literal) must error on nil data")
	}
	if _, err := AsMutableMap(NewTypeLiteral(TMap)); err == nil {
		t.Error("AsMutableMap(type literal) must error on nil data")
	}
}

func TestS7AsFloatApproxBadPayloads(t *testing.T) {
	if _, err := AsFloatApprox(MakeValue(TBigInteger, StrPayload{S: "x"})); err == nil {
		t.Error("AsFloatApprox on an unreadable BigInteger must error")
	}
	if _, err := AsFloatApprox(MakeValue(TBigDecimal, StrPayload{S: "x"})); err == nil {
		t.Error("AsFloatApprox on an unreadable BigDecimal must error")
	}
}

// --- Table / Materializer accessors ---------------------------------------

func TestS7TableMaterializerAccessors(t *testing.T) {
	mpv := NewValueRaw(TList, MaterializerPayload{M: s7Mz{}})
	if _, err := AsTableType(mpv); err != nil {
		t.Errorf("AsTableType(materializer payload): %v", err)
	}
	mzv := NewValueRaw(TList, s7Mz{})
	if _, err := AsTableType(mzv); err != nil {
		t.Errorf("AsTableType(materializer): %v", err)
	}
	// AsList on a materializer: success and error paths.
	if _, err := AsList(mzv); err != nil {
		t.Errorf("AsList(materializer ok): %v", err)
	}
	boomMz := NewValueRaw(TList, s7Mz{err: errS7MzBoom})
	if _, err := AsList(boomMz); err == nil {
		t.Error("AsList on an erroring materializer must fail")
	}
}

var errS7MzBoom = &s7Err{}

// --- kernelFormatDefault marker / instance arms ---------------------------

func TestS7KernelFormatDefaultArms(t *testing.T) {
	// Bounded Type with an unreadable bound -> "Type".
	badBound := NewValueRaw(TType, ChildTypeInfo{Child: MakeValue(nil, IntPayload{N: 1})})
	if got := kernelFormatDefault(badBound); got != "Type" {
		t.Errorf("kernelFormatDefault(bad bound) = %q, want Type", got)
	}
	// ReturnCheck marker.
	rc := NewValueRaw(TReturnCheck, ReturnCheckInfo{FuncName: "f"})
	if got := kernelFormatDefault(rc); got == "" {
		t.Error("returncheck render should not be empty")
	}
	// DefCleanup marker.
	dc := NewValueRaw(TDefCleanup, DefCleanupInfo{})
	if got := kernelFormatDefault(dc); got != "__dc" {
		t.Errorf("defcleanup render = %q, want __dc", got)
	}
	// Micron value backstop.
	micron := MakeValue(TMicron, MicronPayload{Fields: NewOrderedMap()})
	if got := kernelFormatDefault(micron); got == "" {
		t.Error("micron render should not be empty")
	}
}

func TestS7KernelFormatDefaultInstances(t *testing.T) {
	// Class instance: TypeRef present but unnamed -> "Ideal/Class/<id>".
	unnamed := NewClassInstance(TAny, ClassInstanceInfo{
		TypeRef: &ClassTypeInfo{ID: "S7cid", Fields: NewOrderedMap()},
		Fields:  NewOrderedMap(),
	})
	if got := kernelFormatDefault(unnamed); got == "" {
		t.Error("unnamed class instance render should not be empty")
	}
	// Class instance: no TypeRef, Parent present -> leaf name.
	noRef := NewClassInstance(TAny, ClassInstanceInfo{Fields: NewOrderedMap()})
	if got := kernelFormatDefault(noRef); got == "" {
		t.Error("class instance without TypeRef should render via the parent leaf")
	}
	// Class instance: no TypeRef, no Parent -> "Class".
	bare := MakeValue(nil, ClassInstanceInfo{Fields: NewOrderedMap()})
	if got := kernelFormatDefault(bare); got == "" {
		t.Error("bare class instance should render as Class")
	}
	// Resource instance: no TypeRef, Parent present -> leaf name.
	res := NewResourceInstance(TAny, ResourceInstanceInfo{Fields: NewOrderedMap()})
	if got := kernelFormatDefault(res); got == "" {
		t.Error("resource instance without TypeRef should render via the parent leaf")
	}
}

// --- IDPrefixForType nil ---------------------------------------------------

func TestS7IDPrefixForTypeNil(t *testing.T) {
	if got := IDPrefixForType(nil); got != "T_" {
		t.Errorf("IDPrefixForType(nil) = %q, want T_", got)
	}
}
