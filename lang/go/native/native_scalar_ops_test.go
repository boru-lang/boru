package native

import (
	"errors"
	"strings"
	"testing"

	parser "github.com/boru-lang/boru/parser/go"
)

// Within-type scalar & Micron operations (native_scalar_ops.go). The
// end-to-end behaviour is pinned by lang/spec/scalar-micron-ops.tsv; these
// Go tests reach the internal helpers and defensive branches the .tsv rows
// cannot (size guards, direct cross-type projections, the check-mode
// mirror, the nested Map/List recursion, and the reconstruction failures).

func opsReg(t *testing.T) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// runStmt runs source for its effect (e.g. a def), discarding the stack.
func runStmt(t *testing.T, r *Registry, src string) {
	t.Helper()
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, err := NewTop(r).Run(toks); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
}

// evalOneVal runs source expected to leave exactly one value and returns it.
func evalOneVal(t *testing.T, r *Registry, src string) Value {
	t.Helper()
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, err := NewTop(r).Run(toks)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	if len(out) != 1 {
		t.Fatalf("run %q: want 1 value, got %d", src, len(out))
	}
	return out[0]
}

// ---- String / Atom occurrence package ----

func TestSeqOpString(t *testing.T) {
	r := opsReg(t)
	cases := []struct {
		op, a, b, want string
	}{
		{"add", "foo", "bar", "foobar"},
		{"sub", "a-b-c", "-", "abc"},
		{"sub", "abc", "", "abc"},       // empty right operand is a no-op
		{"sub", "abc", "z", "abc"},      // no occurrence
		{"mod", "a/b/c", "/", "c"},      // tail after last
		{"mod", "abc", "/", "abc"},      // no occurrence -> whole
		{"mul", "ab", "xy", "axaybxby"}, // Cartesian
		{"pow", "ab", "xy", "abab"},     // repeat per char
		{"pow", "ab", "", ""},           // empty exponent
	}
	for _, c := range cases {
		v, err := seqOp(r, c.op, c.a, c.b, NewString)
		if err != nil {
			t.Fatalf("seqOp %s %q %q: %v", c.op, c.a, c.b, err)
		}
		got, _ := v.AsConcreteString()
		if got != c.want {
			t.Errorf("seqOp %s %q %q = %q, want %q", c.op, c.a, c.b, got, c.want)
		}
	}
}

func TestSeqOpDivCount(t *testing.T) {
	r := opsReg(t)
	v, err := seqOp(r, "div", "a,b,c", ",", NewString)
	if err != nil {
		t.Fatal(err)
	}
	n, ierr := v.AsConcreteInteger()
	if ierr != nil || n != 2 {
		t.Fatalf("div count = %v (err %v), want 2", n, ierr)
	}
}

func TestSeqOpEmptyDivisorErrors(t *testing.T) {
	r := opsReg(t)
	for _, op := range []string{"div", "mod"} {
		if _, err := seqOp(r, op, "abc", "", NewString); err == nil {
			t.Errorf("seqOp %s by empty string: want error", op)
		}
	}
}

func TestSeqOpAtomWrapper(t *testing.T) {
	r := opsReg(t)
	v, err := seqOp(r, "add", "foo", "bar", NewAtom)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Parent().ConformsTo(TAtom) {
		t.Fatalf("atom seqOp result parent = %s, want Atom", v.Parent().Leaf())
	}
	name, _ := AsAtom(v)
	if name != "foobar" {
		t.Errorf("atom add = %q, want foobar", name)
	}
}

func TestSeqSizeGuard(t *testing.T) {
	r := opsReg(t)
	if err := seqSizeGuard(r, "mul", 10); err != nil {
		t.Errorf("small size guard errored: %v", err)
	}
	if err := seqSizeGuard(r, "mul", int64(maxStringResultBytes)+1); err == nil {
		t.Errorf("oversize guard did not error")
	}
}

func TestSeqMulPowGuardTrips(t *testing.T) {
	r := opsReg(t)
	// Large operands whose product exceeds the cap trip the guard BEFORE
	// materialising the result (the guard is checked first).
	big := strings.Repeat("a", 20000)
	if _, err := seqMul(r, "mul", big, big, NewString); err == nil {
		t.Errorf("seqMul over cap: want error")
	}
	if _, err := seqPow(r, "pow", big, big, NewString); err == nil {
		t.Errorf("seqPow over cap: want error")
	}
	// The guard estimates BYTES, not runes: two inputs of 4-byte runes
	// whose rune-count product passes the old rune-based estimate must
	// still trip on their byte size (PR #292 review).
	wide := strings.Repeat("\U0001F600", 6000) // 6000 runes, 24000 bytes
	if _, err := seqMul(r, "mul", wide, wide, NewString); err == nil {
		t.Errorf("seqMul multibyte over cap: want error")
	}
	// A small multibyte product still succeeds, byte-exactly.
	v, err := seqMul(r, "mul", "é", "xy", NewString)
	if err != nil {
		t.Fatalf("seqMul small multibyte: %v", err)
	}
	if s, _ := v.AsConcreteString(); s != "éxéy" {
		t.Errorf("seqMul multibyte = %q, want éxéy", s)
	}
}

// ---- Bytes occurrence package ----

func TestBytesSeqOp(t *testing.T) {
	r := opsReg(t)
	b := func(s string) []byte { return []byte(s) }
	cases := []struct {
		op, a, bb, want string
	}{
		{"add", "ab", "cd", "abcd"},
		{"sub", "a.b.c", ".", "abc"},
		{"sub", "abc", "", "abc"},
		{"mod", "p/q/r", "/", "r"},
		{"mod", "abc", "/", "abc"},
		{"mul", "ab", "xy", "axaybxby"},
		{"pow", "ab", "xy", "abab"},
	}
	for _, c := range cases {
		v, err := bytesSeqOp(r, c.op, b(c.a), b(c.bb))
		if err != nil {
			t.Fatalf("bytesSeqOp %s: %v", c.op, err)
		}
		got, _ := asBytes(v)
		if string(got) != c.want {
			t.Errorf("bytesSeqOp %s %q %q = %q, want %q", c.op, c.a, c.bb, string(got), c.want)
		}
	}
}

func TestBytesSeqOpDivAndEmpty(t *testing.T) {
	r := opsReg(t)
	v, err := bytesSeqOp(r, "div", []byte("aXaXa"), []byte("X"))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := v.AsConcreteInteger(); n != 2 {
		t.Errorf("bytes div = %d, want 2", n)
	}
	for _, op := range []string{"div", "mod"} {
		if _, err := bytesSeqOp(r, op, []byte("abc"), nil); err == nil {
			t.Errorf("bytes %s by empty: want error", op)
		}
	}
}

func TestBytesMulGuardTrips(t *testing.T) {
	r := opsReg(t)
	big := make([]byte, 20000)
	if _, err := bytesMul(r, "mul", big, big); err == nil {
		t.Errorf("bytesMul over cap: want error")
	}
	if _, err := bytesSeqOp(r, "pow", big, big); err == nil {
		t.Errorf("bytes pow over cap: want error")
	}
}

// ---- Boolean: defined error ----

func TestBooleanArith(t *testing.T) {
	r := opsReg(t)
	h := booleanArithHandler("add")
	if _, err := h([]Value{NewBoolean(true), NewBoolean(false)}, nil, nil, r); err == nil {
		t.Errorf("boolean add handler: want error")
	}
	// no-op outside check mode.
	if out := booleanArithReturns("add")([]Value{NewBoolean(true), NewBoolean(false)}, r); len(out) != 0 {
		t.Errorf("returns outside check mode: want empty")
	}
	// check-mode mirror emits a diagnostic and no residual.
	done := r.Check.Begin()
	out := booleanArithReturns("add")([]Value{NewBoolean(true), NewBoolean(false)}, r)
	if len(out) != 0 {
		t.Errorf("boolean check-mode residual = %d, want 0", len(out))
	}
	if len(r.Check.Diagnostics) == 0 {
		t.Errorf("boolean check-mode: no diagnostic emitted")
	}
	// wrong arity is a no-op even in check mode.
	if out := booleanArithReturns("add")([]Value{}, r); len(out) != 0 {
		t.Errorf("returns with wrong arity: want empty")
	}
	done()
}

// ---- applyScalarBinaryOp dispatch + cross-type ----

func TestApplyScalarBinaryOpFamilies(t *testing.T) {
	r := opsReg(t)
	// Number
	v, err := applyScalarBinaryOp(r, "add", NewInteger(2), NewInteger(3))
	if err != nil || opsInt(t, v) != 5 {
		t.Fatalf("number add = %v (err %v)", v, err)
	}
	// String
	v, _ = applyScalarBinaryOp(r, "add", NewString("a"), NewString("b"))
	if s, _ := v.AsConcreteString(); s != "ab" {
		t.Fatalf("string add = %q", s)
	}
	// Atom
	v, _ = applyScalarBinaryOp(r, "add", NewAtom("a"), NewAtom("b"))
	if s, _ := AsAtom(v); s != "ab" {
		t.Fatalf("atom add = %q", s)
	}
	// Bytes
	v, _ = applyScalarBinaryOp(r, "add", newBytes([]byte("a")), newBytes([]byte("b")))
	if got, _ := asBytes(v); string(got) != "ab" {
		t.Fatalf("bytes add = %q", string(got))
	}
	// Boolean -> error
	if _, err := applyScalarBinaryOp(r, "add", NewBoolean(true), NewBoolean(false)); err == nil {
		t.Fatalf("boolean add: want error")
	}
	// Map (nested) -> field-wise
	m1 := NewOrderedMap()
	m1.Set("x", NewInteger(1))
	m2 := NewOrderedMap()
	m2.Set("x", NewInteger(4))
	m2.Set("y", NewInteger(9))
	v, err = applyScalarBinaryOp(r, "add", NewMap(m1), NewMap(m2))
	if err != nil {
		t.Fatalf("map add: %v", err)
	}
	rm, _ := AsMap(v)
	if xv, _ := rm.Get("x"); opsInt(t, xv) != 5 {
		t.Fatalf("map add x = %v", xv)
	}
	if yv, _ := rm.Get("y"); opsInt(t, yv) != 9 { // right-only key passes through
		t.Fatalf("map add y = %v", yv)
	}
	// List (nested) -> element-wise with tail pass-through
	l1 := NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})
	l2 := NewList([]Value{NewInteger(10), NewInteger(20)})
	v, _ = applyScalarBinaryOp(r, "add", l1, l2)
	rl, _ := AsList(v)
	if rl.Len() != 3 || opsInt(t, rl.Get(0)) != 11 || opsInt(t, rl.Get(2)) != 3 {
		t.Fatalf("list add = %v", v)
	}
	// left list longer already covered; now right longer
	v, _ = applyScalarBinaryOp(r, "add", l2, l1)
	rl, _ = AsList(v)
	if rl.Len() != 3 || opsInt(t, rl.Get(2)) != 3 {
		t.Fatalf("list add (right longer) = %v", v)
	}
}

func TestApplyScalarBinaryOpAllNumericOps(t *testing.T) {
	r := opsReg(t)
	cases := []struct {
		op   string
		a, b int64
		want int64
	}{
		{"sub", 10, 3, 7}, {"mul", 4, 5, 20}, {"div", 10, 3, 3},
		{"mod", 10, 3, 1}, {"pow", 2, 10, 1024},
	}
	for _, c := range cases {
		v, err := applyScalarBinaryOp(r, c.op, NewInteger(c.a), NewInteger(c.b))
		if err != nil || opsInt(t, v) != c.want {
			t.Errorf("applyScalarBinaryOp %s %d %d = %v (err %v), want %d", c.op, c.a, c.b, v, err, c.want)
		}
	}
	// a numeric field op that faults (div by zero) propagates the error.
	if _, err := applyScalarBinaryOp(r, "div", NewInteger(1), NewInteger(0)); err == nil {
		t.Errorf("div by zero: want error")
	}
}

func TestApplyScalarBinaryOpCrossTypeAndNonConcrete(t *testing.T) {
	r := opsReg(t)
	// cross-type
	if _, err := applyScalarBinaryOp(r, "add", NewInteger(1), NewString("x")); err == nil {
		t.Errorf("cross-type: want error")
	}
	// non-concrete operand
	if _, err := applyScalarBinaryOp(r, "add", NewTypeLiteral(TInteger), NewInteger(1)); err == nil {
		t.Errorf("non-concrete: want error")
	}
}

func TestMapListFieldwiseErrors(t *testing.T) {
	r := opsReg(t)
	// non-concrete map
	if _, err := mapFieldwiseOp(r, "add", NewTypeLiteral(TMap), NewMap(NewOrderedMap())); err == nil {
		t.Errorf("non-concrete map: want error")
	}
	// inner error (shared Boolean key)
	m1 := NewOrderedMap()
	m1.Set("f", NewBoolean(true))
	m2 := NewOrderedMap()
	m2.Set("f", NewBoolean(false))
	if _, err := mapFieldwiseOp(r, "add", NewMap(m1), NewMap(m2)); err == nil {
		t.Errorf("map inner error: want error")
	}
	// left-only key branch
	m3 := NewOrderedMap()
	m3.Set("only", NewInteger(7))
	res, err := mapFieldwiseOp(r, "add", NewMap(m3), NewMap(NewOrderedMap()))
	if err != nil {
		t.Fatal(err)
	}
	rm, _ := AsMap(res)
	if v, _ := rm.Get("only"); opsInt(t, v) != 7 {
		t.Errorf("map left-only key dropped")
	}
	// non-concrete list
	if _, err := listElementwiseOp(r, "add", NewTypeLiteral(TList), NewList(nil)); err == nil {
		t.Errorf("non-concrete list: want error")
	}
	// list inner error
	if _, err := listElementwiseOp(r, "add",
		NewList([]Value{NewBoolean(true)}), NewList([]Value{NewBoolean(false)})); err == nil {
		t.Errorf("list inner error: want error")
	}
}

// ---- Micron ops ----

func TestMicronFieldwiseAndErrors(t *testing.T) {
	r := opsReg(t)
	// user kind field-wise
	runStmt(t, r, `def Pton refine Micron {x:Integer y:Integer}`)
	p1 := evalOneVal(t, r, `make Pton {x:1 y:2}`)
	p2 := evalOneVal(t, r, `make Pton {x:10 y:20}`)
	sum, err := micronBinaryOp(r, "add", p1, p2)
	if err != nil {
		t.Fatalf("micron field-wise add: %v", err)
	}
	if sum.String() != "{x:11 y:22}" {
		t.Fatalf("field-wise add = %q, want {x:11 y:22}", sum.String())
	}
	// cross-kind
	e := evalOneVal(t, r, `make Emailon "a@b.io"`)
	if _, err := micronBinaryOp(r, "add", p1, e); err == nil {
		t.Errorf("cross-kind: want error")
	}
	// field op error, annotated with the field name
	runStmt(t, r, `def Flagon refine Micron {f:Boolean}`)
	f1 := evalOneVal(t, r, `make Flagon {f:true}`)
	f2 := evalOneVal(t, r, `make Flagon {f:false}`)
	_, ferr := micronBinaryOp(r, "add", f1, f2)
	if ferr == nil || !strings.Contains(ferr.Error(), `field "f"`) {
		t.Fatalf("field error not annotated: %v", ferr)
	}
	// reconstruction failure (Coloron alpha overflow)
	c1 := evalOneVal(t, r, `make Coloron "#102030"`)
	c2 := evalOneVal(t, r, `make Coloron "#010203"`)
	if _, err := micronBinaryOp(r, "add", c1, c2); err == nil {
		t.Errorf("coloron overflow: want error")
	}
	// right-only optional field passes through (left Urlon has no port).
	u1 := evalOneVal(t, r, `make Urlon "http://a.com/x"`)
	u2 := evalOneVal(t, r, `make Urlon "http://b.com:8080/y"`)
	uj, err := micronBinaryOp(r, "add", u1, u2)
	if err != nil {
		t.Fatalf("urlon field-wise add: %v", err)
	}
	if p, ok := basicMicronProperty(uj, "port"); !ok || opsInt(t, p) != 8080 {
		t.Fatalf("urlon right-only port not passed through: %v (%v)", p, ok)
	}
	// left-only optional field passes through (left Urlon has the port).
	lj, lerr := micronBinaryOp(r, "add", u2, u1)
	if lerr != nil {
		t.Fatalf("urlon left-port add: %v", lerr)
	}
	if p, ok := basicMicronProperty(lj, "port"); !ok || opsInt(t, p) != 8080 {
		t.Fatalf("urlon left-only port not passed through: %v (%v)", p, ok)
	}
	// applyScalarBinaryOp routes a Micron pair through micronBinaryOp.
	em := evalOneVal(t, r, `make Emailon "a@b.io"`)
	if _, err := applyScalarBinaryOp(r, "add", em, em); err != nil {
		t.Fatalf("applyScalarBinaryOp micron pair: %v", err)
	}
}

func TestQionOp(t *testing.T) {
	r := opsReg(t)
	a := evalOneVal(t, r, `make Qion "USD 12.50"`)
	b := evalOneVal(t, r, `make Qion "USD 2.25"`)
	sum, err := micronBinaryOp(r, "add", a, b)
	if err != nil || sum.String() != "USD 14.75" {
		t.Fatalf("qion add = %q (err %v)", sum.String(), err)
	}
	diff, _ := micronBinaryOp(r, "sub", a, b)
	if diff.String() != "USD 10.25" {
		t.Fatalf("qion sub = %q", diff.String())
	}
	// different currency
	e := evalOneVal(t, r, `make Qion "EUR 1.00"`)
	if _, err := micronBinaryOp(r, "add", a, e); err == nil {
		t.Errorf("qion cross-currency: want error")
	}
	// mul / div / mod / pow are defined errors
	for _, op := range []string{"mul", "div", "mod", "pow"} {
		if _, err := micronBinaryOp(r, op, a, b); err == nil {
			t.Errorf("qion %s: want error", op)
		}
	}
}

func TestPathonOp(t *testing.T) {
	r := opsReg(t)
	p := evalOneVal(t, r, `make Pathon "usr/local"`)
	q := evalOneVal(t, r, `make Pathon "bin/boru"`)
	j, err := micronBinaryOp(r, "add", p, q)
	if err != nil || j.String() != "usr/local/bin/boru" {
		t.Fatalf("pathon join = %q (err %v)", j.String(), err)
	}
	abs := evalOneVal(t, r, `make Pathon "/bin"`)
	if _, err := micronBinaryOp(r, "add", p, abs); err == nil {
		t.Errorf("pathon join absolute rhs: want error")
	}
	// sub: strip suffix, no-match, absolute rhs error
	full := evalOneVal(t, r, `make Pathon "usr/local/bin"`)
	suf := evalOneVal(t, r, `make Pathon "local/bin"`)
	s, _ := micronBinaryOp(r, "sub", full, suf)
	if s.String() != "usr" {
		t.Fatalf("pathon sub = %q", s.String())
	}
	nomatch := evalOneVal(t, r, `make Pathon "zzz"`)
	s2, _ := micronBinaryOp(r, "sub", full, nomatch)
	if s2.String() != "usr/local/bin" {
		t.Fatalf("pathon sub no-match = %q", s2.String())
	}
	if _, err := micronBinaryOp(r, "sub", full, abs); err == nil {
		t.Errorf("pathon sub absolute rhs: want error")
	}
	for _, op := range []string{"mul", "div", "mod", "pow"} {
		if _, err := micronBinaryOp(r, op, p, q); err == nil {
			t.Errorf("pathon %s: want error", op)
		}
	}
}

func TestStripSegSuffix(t *testing.T) {
	eq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	if got := stripSegSuffix([]string{"a", "b", "c"}, []string{"b", "c"}); !eq(got, []string{"a"}) {
		t.Errorf("suffix strip = %v", got)
	}
	if got := stripSegSuffix([]string{"a", "b"}, []string{"x"}); !eq(got, []string{"a", "b"}) {
		t.Errorf("non-match = %v", got)
	}
	if got := stripSegSuffix([]string{"a"}, nil); !eq(got, []string{"a"}) {
		t.Errorf("empty suffix = %v", got)
	}
	if got := stripSegSuffix([]string{"a"}, []string{"a", "b"}); !eq(got, []string{"a"}) {
		t.Errorf("longer suffix = %v", got)
	}
}

// ---- handlers, returns, helpers ----

func TestOpHandlersDirectErrors(t *testing.T) {
	r := opsReg(t)
	// pass non-matching types straight to the handlers to hit their guards.
	if _, err := stringOpHandler("sub")([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r); err == nil {
		t.Errorf("stringOpHandler with non-strings: want error")
	}
	if _, err := atomOpHandler("sub")([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r); err == nil {
		t.Errorf("atomOpHandler with non-atoms: want error")
	}
	if _, err := bytesOpHandler("sub")([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r); err == nil {
		t.Errorf("bytesOpHandler with non-bytes: want error")
	}
	// happy paths through the handlers (infix form: args[1] OP args[0]).
	out, err := stringOpHandler("sub")([]Value{NewString("."), NewString("a.b")}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("stringOpHandler happy: %v", err)
	}
	if s, _ := out[0].AsConcreteString(); s != "ab" {
		t.Errorf("stringOpHandler sub = %q", s)
	}
	aout, aerr := atomOpHandler("add")([]Value{NewAtom("b"), NewAtom("a")}, nil, nil, r)
	if aerr != nil {
		t.Fatalf("atomOpHandler happy: %v", aerr)
	}
	if name, _ := AsAtom(aout[0]); name != "ab" {
		t.Errorf("atomOpHandler add = %q", name)
	}
	bout, berr := bytesOpHandler("sub")([]Value{newBytes([]byte(".")), newBytes([]byte("a.b"))}, nil, nil, r)
	if berr != nil {
		t.Fatalf("bytesOpHandler happy: %v", berr)
	}
	if got, _ := asBytes(bout[0]); string(got) != "ab" {
		t.Errorf("bytesOpHandler sub = %q", string(got))
	}
	// micron handler happy path (Emailon field-wise).
	e := evalOneVal(t, r, `make Emailon "a@b.io"`)
	mout, merr := micronOpHandler("add")([]Value{e, e}, nil, nil, r)
	if merr != nil || len(mout) != 1 {
		t.Fatalf("micronOpHandler happy: %v", merr)
	}
}

func TestSeqReturnsAndMicronReturns(t *testing.T) {
	if got := seqReturnType("div", TString); got != TInteger {
		t.Errorf("seqReturnType div = %v", got)
	}
	if got := seqReturnType("sub", TString); got != TString {
		t.Errorf("seqReturnType sub = %v", got)
	}
	// seqOpReturns: concrete operands mirror a guaranteed error in check
	// mode; the residual is the declared type either way.
	r := opsReg(t)
	h := stringOpHandler("div")
	rf := seqOpReturns("div", TString, h)
	done := r.Check.Begin()
	out := rf([]Value{NewString(""), NewString("abc")}, r) // "abc" div "" — guaranteed error
	if len(out) != 1 || !out[0].Parent().Equal(TInteger) {
		t.Errorf("seqOpReturns residual = %v", out)
	}
	if len(r.Check.Diagnostics) == 0 {
		t.Errorf("seqOpReturns: guaranteed empty-divisor error not mirrored")
	}
	nBefore := len(r.Check.Diagnostics)
	if out := rf([]Value{NewString(","), NewString("a,b")}, r); len(out) != 1 { // fine — no diagnostic
		t.Errorf("seqOpReturns ok-path residual = %v", out)
	}
	if len(r.Check.Diagnostics) != nBefore {
		t.Errorf("seqOpReturns: ok path emitted a diagnostic")
	}
	done()
	// micronOpReturns: same-kind carrier narrows to the kind.
	e := evalOneVal(t, r, `make Emailon "a@b.io"`)
	mrf := micronOpReturns("add")
	c := mrf([]Value{e, e}, r)
	if len(c) != 1 || !c[0].Parent().Equal(TEmailon) {
		t.Errorf("micronOpReturns same-kind = %v", c)
	}
	// different kinds -> family root; CARRIER operands are NOT mirrored
	// (a carrier's runtime tag can be a strict subtype a more-specific
	// user overload claims — the refinement escape).
	u := evalOneVal(t, r, `make Urlon "https://x.com"`)
	done = r.Check.Begin()
	nBefore = len(r.Check.Diagnostics)
	c = mrf([]Value{NewCarrier(TEmailon), NewCarrier(TUrlon)}, r)
	if len(c) != 1 || !c[0].Parent().Equal(TMicron) {
		t.Errorf("micronOpReturns cross-kind = %v", c)
	}
	if len(r.Check.Diagnostics) != nBefore {
		t.Errorf("micronOpReturns: carrier operands must not be mirrored")
	}
	if micronOpReturns("mul")([]Value{NewCarrier(TQion), NewCarrier(TQion)}, r); len(r.Check.Diagnostics) != nBefore {
		t.Errorf("micronOpReturns: Qion carrier operands must not be mirrored")
	}
	// two CONCRETE operands run the full handler (cross-kind pair errors).
	micronOpReturns("sub")([]Value{e, u}, r)
	if len(r.Check.Diagnostics) <= nBefore {
		t.Errorf("micronOpReturns concrete cross-kind: no mirror")
	}
	done()
	// wrong arity -> family root, no mirror.
	c = mrf([]Value{e}, r)
	if len(c) != 1 || !c[0].Parent().Equal(TMicron) {
		t.Errorf("micronOpReturns arity = %v", c)
	}
}

func TestHelperEdges(t *testing.T) {
	if typeLeaf(Value{}) != "?" {
		t.Errorf("typeLeaf nil parent")
	}
	if boruErrorDetail(errors.New("plain")) != "plain" {
		t.Errorf("boruErrorDetail non-boru")
	}
	if boruErrorDetail(&BoruError{Detail: "d"}) != "d" {
		t.Errorf("boruErrorDetail boru")
	}
	if _, ok := towerOpsFor("add"); !ok {
		t.Errorf("towerOpsFor add missing")
	}
}

// ---- helpers ----

func opsInt(t *testing.T, v Value) int64 {
	t.Helper()
	n, err := v.AsConcreteInteger()
	if err != nil {
		t.Fatalf("not an integer: %v", v)
	}
	return n
}
