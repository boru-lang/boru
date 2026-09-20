package native

import (
	"strings"
	"testing"

	parser "github.com/boru-lang/boru/parser/go"
)

// Wave-3 coverage for native_type.go: refine (plain / bare / gen-schema),
// table, enum, teq/tis, guard, tany/tall, tpartial, and the convert
// scalar family (BigInteger / BigDecimal edges). Rejections are paired
// with each acceptance per the test discipline.

// w3TypeReg returns a default registry with the moved module words
// (tpartial, …) registered bare.
func w3TypeReg() *Registry {
	r, _ := DefaultRegistry()
	registerIOWords(r)
	return r
}

func w3Type(t *testing.T, src string) (string, error) {
	t.Helper()
	return w3Run(t, w3TypeReg(), src)
}

func w3TypeWant(t *testing.T, src, want string) {
	t.Helper()
	got, err := w3Type(t, src)
	if err != nil {
		t.Fatalf("%q: unexpected error: %v", src, err)
	}
	if got != want {
		t.Errorf("%q = %q, want %q", src, got, want)
	}
}

func w3TypeErr(t *testing.T, src, substr string) {
	t.Helper()
	_, err := w3Type(t, src)
	if err == nil {
		t.Fatalf("%q: expected error containing %q, got none", src, substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("%q: error %q does not contain %q", src, err.Error(), substr)
	}
}

// ---- refine: plain construction and rejections ----

func TestW3RefineRecordAndTable(t *testing.T) {
	// Record construction, then a Table over it.
	w3TypeWant(t, `def R refine Record [a:Integer]  (make R {a:1}) dot a`, `1`)
	got, err := w3Type(t, `def T refine Table (refine Record [a:Integer])  T istype`)
	if err != nil {
		t.Fatalf("refine Table: %v", err)
	}
	if got != `true` {
		t.Errorf("refine Table istype = %q, want true", got)
	}
	// Rejections: a record takes a LIST of pairs; a non-record table arg;
	// no subtyping for records and tables.
	w3TypeErr(t, `refine Record {a:1}`, "takes a list of field pairs")
	if _, terr := tableHandler([]Value{NewInteger(5)}, nil, nil, nil); terr == nil ||
		!strings.Contains(terr.Error(), "must be a record type") {
		t.Errorf("table over non-record: got %v", terr)
	}
	w3TypeErr(t, `def R refine Record [a:Integer]  refine R [b:String]`,
		"no subtyping")
	w3TypeErr(t, `def R refine Record [a:Integer]  def T refine Table R  refine T R`,
		"no subtyping")
	// An unknown base is rejected with the pinned message.
	w3TypeErr(t, `refine 5 {}`, "base must be Record, Table, or a class type")
	// The old `refine Object {…}` class form is retired with a hint
	// (direct call: no surface name binds the bare Object/Class literal).
	r := w3TypeReg()
	if obj := r.Ideals.Get("Object"); obj != nil && obj.Construct != nil {
		_, oerr := obj.Construct(NewTypeLiteral(TClass), NewMap(NewOrderedMap()), r)
		if oerr == nil || !strings.Contains(oerr.Error(), "no longer the class form") {
			t.Errorf("refine bare-Object: got %v", oerr)
		}
	} else {
		t.Error("Object ideal not registered")
	}
	// A class refine still constructs a subtype.
	w3TypeWant(t, `def P class {x:Integer}  def Q refine P {y:Integer}  (make Q {x:1 y:2}) dot y`, `2`)
}

func TestW3RefineBare(t *testing.T) {
	// Bare nominal subtype: a distinct lattice node below the base.
	w3TypeWant(t, `def Foo refine Integer  Foo teq Integer`, `false`)
	// A non-type 1-arg refine is rejected.
	w3TypeErr(t, `refine 5`, "argument must be a type")
	// A non-bare type body passes through unchanged (predicate base).
	w3TypeWant(t, `def Pos (refine (Integer gte 0))  5 is Pos`, `true`)
}

func TestW3RefineGenSchema(t *testing.T) {
	// The pending-gen path wraps the record as a schema.
	w3TypeWant(t,
		`def Box gen [T] refine Record [value:T] end  (Box of [Integer]) istype`,
		`true`)
	// A gen over a non-Record construction is unsupported.
	w3TypeErr(t,
		`def R refine Record [a:Integer]  def X gen [T] refine Table R end`,
		"gen")
	// A gen whose construction itself errors pops the bindings and raises.
	w3TypeErr(t, `def X gen [T] refine 5 {} end`,
		"base must be Record, Table, or a class type")
}

// ---- enum ----

func TestW3Enum(t *testing.T) {
	w3TypeWant(t, `def Color enum [red green blue]  red/q is Color`, `true`)
	w3TypeWant(t, `def Color enum [red green blue]  mauve/q is Color`, `false`)
	// Child-typed enum validates each element.
	w3TypeWant(t, `def Small enum [:Integer 1 2 3]  2 is Small`, `true`)
	w3TypeErr(t, `enum [:Integer 1 'x']`, "does not satisfy child type")
	// Non-concrete arg (direct call — dispatch gates the literal away).
	if _, err := enumHandler([]Value{NewTypeLiteral(TList)}, nil, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "concrete list") {
		t.Errorf("enum over List literal: got %v", err)
	}
	// enumReturns: concrete args fold to the real enum; a carrier falls
	// back to the bare TEnum carrier.
	out := enumReturns([]Value{NewList([]Value{NewAtom("a")})}, nil)
	if len(out) != 1 || !IsDisjunct(out[0]) {
		t.Errorf("enumReturns concrete = %v, want an enum value", out)
	}
	out = enumReturns([]Value{NewCarrier(TList)}, nil)
	if len(out) != 1 || !out[0].Parent.Equal(TEnum) {
		t.Errorf("enumReturns carrier = %v, want TEnum carrier", out)
	}
}

// ---- typeof (check-mode refinement) ----

func TestW3TypeofReturns(t *testing.T) {
	r := w3TypeReg()
	r.Check.Mode = true
	toks, err := parser.Parse(`def One (typeof (const 1))  end  1 is One`)
	if err != nil {
		t.Fatal(err)
	}
	if _, runErr := NewTop(r).Run(toks); runErr != nil {
		t.Fatalf("check typeof: %v", runErr)
	}
	// Direct: a carrier arg keeps the bare Type carrier.
	out := typeofReturns([]Value{NewCarrier(TInteger)}, r)
	if len(out) != 1 || !out[0].Parent.Equal(TType) {
		t.Errorf("typeofReturns carrier = %v, want Type carrier", out)
	}
}

// ---- teq / tis ----

func TestW3Teq(t *testing.T) {
	w3TypeWant(t, `Integer teq Integer`, `true`)
	w3TypeWant(t, `Integer teq Number`, `false`)
	w3TypeWant(t, `5 teq Integer`, `false`) // a value is not a type
	w3TypeWant(t, `(Integer tor String) teq (String tor Integer)`, `true`)
}

func TestW3Tis(t *testing.T) {
	w3TypeWant(t, `5 tis Integer`, `true`)
	w3TypeWant(t, `5 tis Number`, `true`)
	w3TypeWant(t, `5 tis String`, `false`)
	w3TypeWant(t, `Integer tis Number`, `true`)
	// tis is TAG-ONLY: the predicate does not run.
	w3TypeWant(t, `5 tis (Integer gt 10)`, `true`)
	// A bare-refine newtype tag only matches its own chain.
	w3TypeWant(t, `def Pos refine Integer  2 tis Pos`, `false`)
}

// ---- guard ----

func TestW3Guard(t *testing.T) {
	w3TypeWant(t, `true guard 42`, `42`)
	w3TypeWant(t, `false guard 42`, `None`)
	// Non-concrete condition (direct call).
	if _, err := guardHandler([]Value{NewInteger(1), NewTypeLiteral(TBoolean)}, nil, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "condition must be Boolean") {
		t.Errorf("guard with Boolean literal: got %v", err)
	}
	// guardReturns: the join of value|None, and the empty-args arm.
	out := guardReturns([]Value{NewCarrier(TInteger), NewCarrier(TBoolean)}, nil)
	if len(out) != 1 {
		t.Fatalf("guardReturns = %v", out)
	}
	out = guardReturns(nil, nil)
	if len(out) != 1 || !out[0].Parent.Equal(TAny) {
		t.Errorf("guardReturns() = %v, want Any carrier", out)
	}
}

// ---- tany / tall ----

func TestW3TanyTall(t *testing.T) {
	w3TypeWant(t, `tany []`, `Never`)
	w3TypeWant(t, `tany [Integer]`, `Integer`)
	w3TypeWant(t, `tany ['a' 'b']`, `'a' tor 'b'`)
	w3TypeWant(t, `tall []`, `Any`)
	w3TypeWant(t, `tall [Integer Number]`, `Integer`)
	w3TypeWant(t, `tall ['a' 'b']`, `Never`)
	// Duplicate alternatives simplify to one.
	w3TypeWant(t, `tany [Integer Integer]`, `Integer`)
	// Type-literal rejection (direct calls).
	r := w3TypeReg()
	if _, err := tanyHandler([]Value{NewTypeLiteral(TList)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "concrete list") {
		t.Errorf("tany over List literal: got %v", err)
	}
	if _, err := tallHandler([]Value{NewTypeLiteral(TList)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "concrete list") {
		t.Errorf("tall over List literal: got %v", err)
	}
}

func TestW3TypeAlgebraListFoldCheck(t *testing.T) {
	// Check mode: a fully-static list folds to the real result; a disjunct
	// result rides as a dynamic carrier.
	r := w3TypeReg()
	r.Check.Mode = true
	for _, src := range []string{
		`tany [Integer String]`, // disjunct result → dynamic carrier
		`tall ['a' 'b']`,        // Never result → concrete
		`tany [(1 add 2)]`,      // carrier element → dyn fallback
	} {
		toks, err := parser.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		out, runErr := NewTop(r).Run(toks)
		if runErr != nil {
			t.Fatalf("check %q: %v", src, runErr)
		}
		if len(out) == 0 {
			t.Errorf("check %q: expected a result", src)
		}
	}
	// Direct: a non-concrete arg keeps the declared dynamic(Any).
	fold := typeAlgebraListFold(tanyHandler)
	out := fold([]Value{NewCarrier(TList)}, r)
	if len(out) != 1 || !out[0].Dynamic {
		t.Errorf("typeAlgebraListFold carrier = %v, want dynamic Any", out)
	}
}

// ---- tpartial ----

func TestW3Tpartial(t *testing.T) {
	// Record: every field becomes T|None; the partial admits a missing key.
	w3TypeWant(t, `(tpartial (refine Record [x:Integer])) istype`, `true`)
	// Idempotent over an already-optional field.
	w3TypeWant(t, `(tpartial (tpartial (refine Record [x:Integer]))) istype`, `true`)
	// Class: flattens fields into a fresh anonymous class type.
	w3TypeWant(t, `def P class {x:Integer}  (tpartial P) istype`, `true`)
	// Rejection.
	w3TypeErr(t, `tpartial 5`, "must be a Record or Object type")
}

func TestW3MakeOptionalTypeNone(t *testing.T) {
	// A None field is left unchanged (the IsNoneShape arm).
	got := makeOptionalType(NewTypeLiteral(TNone))
	if !IsNoneShape(got) {
		t.Errorf("makeOptionalType(None) = %v, want None", got)
	}
}

// ---- convert: big numbers and scalar edges ----

func TestW3ConvertBigInteger(t *testing.T) {
	w3TypeWant(t, `convert BigInteger 42`, `0d42`)
	w3TypeWant(t, `convert BigInteger "12345678901234567890123"`, `0d12345678901234567890123`)
	w3TypeWant(t, `convert BigInteger 0d7`, `0d7`)
	// BigDecimal truncates toward zero.
	w3TypeWant(t, `convert BigInteger (convert BigDecimal "7.9")`, `0d7`)
	w3TypeWant(t, `convert BigInteger (convert BigDecimal "-7.9")`, `-0d7`)
	// A Float is declined (inexact).
	w3TypeErr(t, `convert BigInteger 1.5`, "Float")
	w3TypeErr(t, `convert BigInteger "zz"`, "cannot convert")
}

func TestW3ConvertBigDecimal(t *testing.T) {
	w3TypeWant(t, `convert BigDecimal 42`, `0d42`)
	w3TypeWant(t, `convert BigDecimal 0d7`, `0d7`)
	w3TypeWant(t, `convert BigDecimal "1.25"`, `0d1.25`)
	w3TypeErr(t, `convert BigDecimal 1.5`, "Float")
	w3TypeErr(t, `convert BigDecimal "zz"`, "cannot convert")
	// A base option is not supported for BigDecimal.
	if _, err := convertToBigDecimal(NewString("1"), "hex"); err == nil ||
		!strings.Contains(err.Error(), "not supported for BigDecimal") {
		t.Errorf("convertToBigDecimal base: got %v", err)
	}
}

func TestW3ConvertBigToInteger(t *testing.T) {
	// Big → Integer round trips within int64 range.
	w3TypeWant(t, `convert Integer 0d42`, `42`)
	w3TypeWant(t, `convert Integer (convert BigDecimal "7.9")`, `7`)
	// Overflow is loud.
	w3TypeErr(t, `convert Integer 0d123456789012345678901234567890`, "overflows Integer")
	w3TypeErr(t, `convert Integer (convert BigDecimal "123456789012345678901234567890")`, "overflows Integer")
	// BigInteger → Float is the lossy projection.
	w3TypeWant(t, `convert Float 0d3`, `3.0`)
}

func TestW3ConvertScalarEdges(t *testing.T) {
	w3TypeWant(t, `convert Integer 3.7`, `3`)   // truncation
	w3TypeWant(t, `convert Integer "12"`, `12`) // string parse
	w3TypeErr(t, `convert Integer "x"`, "cannot convert")
	w3TypeWant(t, `convert Boolean 'true'`, `true`)
	w3TypeWant(t, `convert Boolean 'false'`, `true`) // content not parsed — non-empty String is true
	w3TypeWant(t, `convert Boolean 'other'`, `true`)
	w3TypeWant(t, `convert Boolean 0`, `false`)
	w3TypeWant(t, `convert Atom 'zed'`, `zed/q`)
	w3TypeWant(t, `convert String {base:'hex'} 255`, `'ff'`)
	w3TypeWant(t, `convert String {base:'HEX'} 255`, `'FF'`)
	w3TypeWant(t, `convert String {base:'bin'} 5`, `'101'`)
	w3TypeWant(t, `convert String {base:'oct'} 8`, `'10'`)
	w3TypeErr(t, `convert String {base:'nope'} 8`, "unknown base")
	w3TypeErr(t, `convert String {base:'hex'} 1.5`, "only supported for integer")
	w3TypeWant(t, `convert Integer {base:'hex'} 'ff'`, `255`)
	w3TypeWant(t, `convert Integer {base:'bin'} '101'`, `5`)
	w3TypeWant(t, `convert Integer {base:'oct'} '10'`, `8`)
	w3TypeErr(t, `convert Integer {base:'nope'} '1'`, "unknown base")
	w3TypeErr(t, `convert Integer {base:'hex'} 'zz'`, "cannot convert")
}

func TestW3ConvertIdeal(t *testing.T) {
	// An Ideal converts to Map or List.
	got, err := w3Type(t, `convert Map (module [export "M" {a:1}])`)
	if err != nil {
		t.Fatalf("convert Map module: %v", err)
	}
	if !strings.Contains(got, "{") {
		t.Errorf("convert Map module = %q, want a map", got)
	}
	// A populated first arg is rejected (direct call: dispatch requires a
	// type literal in the TypeArgs slot).
	r := w3TypeReg()
	if _, err := convertIdealHandler([]Value{NewList(nil), NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "must be a type literal") {
		t.Errorf("convertIdealHandler populated target: got %v", err)
	}
}

// ---- is: class-tag and surface arms ----

func TestW3IsBranches(t *testing.T) {
	// Class-type RHS: tag identity for instances.
	w3TypeWant(t, `def P class {x:Integer}  (make P {x:1}) is P`, `true`)
	w3TypeWant(t, `def P class {x:Integer}  def Q class {y:Integer}  (make Q {y:1}) is P`, `false`)
	// Table RHS.
	w3TypeWant(t, `def R refine Record [a:Integer]  def T refine Table R  5 is T`, `false`)
	// Predicate RHS runs the function (direct call: forward collection
	// treats a Function token as a boundary, so the arm is pinned here).
	r := w3TypeReg()
	toks, perr := parser.Parse(`def positive fn [[n:Integer] [Boolean] [n gt 0]]  positive/v`)
	if perr != nil {
		t.Fatal(perr)
	}
	out, rerr := NewTop(r).Run(toks)
	if rerr != nil || len(out) != 1 {
		t.Fatalf("building predicate fn: %v %v", out, rerr)
	}
	fnVal := out[0]
	got, err := isHandler([]Value{fnVal, NewInteger(4)}, nil, nil, r)
	if err != nil || Canon(got) != `true` {
		t.Errorf("4 is positive = %v (%v), want true", Canon(got), err)
	}
	got, err = isHandler([]Value{fnVal, NewInteger(-4)}, nil, nil, r)
	if err != nil || Canon(got) != `false` {
		t.Errorf("-4 is positive = %v (%v), want false", Canon(got), err)
	}
}

// ---- base ----

func TestW3Base(t *testing.T) {
	// `base` yields the base VALUE of the family (Integer → 0).
	w3TypeWant(t, `base 5`, `0`)
	w3TypeWant(t, `base 'x'`, `''`)
	// A type literal IS its own node (the IsBareTypeNode arm).
	w3TypeWant(t, `def Pos refine Integer  base Pos`, `0`)
}
