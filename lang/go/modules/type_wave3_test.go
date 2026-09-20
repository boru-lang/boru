package modules

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// Wave-3 boru:type-util coverage: record surgery over CLASS types (the
// fieldsOf / constructRecordOrObjectLike object branches), function-shape
// introspection over fnsig values (the FnUndefInfo branch), declared-return
// introspection (returnsofResult single / multiple), field-name decoding
// edges, disjunct stripping edges, and the structural-equality fallback of
// the type-set algebra — positives paired with the matching compile failures.

// runTypeErr runs expr against a fresh type-module registry and requires
// an error containing want.
func runTypeErr(t *testing.T, expr, want string) {
	t.Helper()
	r := typeRegistry(t)
	values, err := parser.Parse(expr)
	if err != nil {
		t.Fatalf("parse %q: %v", expr, err)
	}
	_, err = native.NewTop(r).Run(values)
	if err == nil {
		t.Fatalf("%s: expected error, got nil", expr)
	}
	if want != "" && !strings.Contains(err.Error(), want) {
		t.Fatalf("%s: error %q does not contain %q", expr, err.Error(), want)
	}
}

// TestTypeClassSurgeryWave3 pins pick / omit / required / merge over CLASS
// types: fieldsOf takes the ClassType branch and the result is rebuilt as
// a fresh object type (constructRecordOrObjectLike's non-record arm).
func TestTypeClassSurgeryWave3(t *testing.T) {
	cases := []struct{ expr, wantPrefix, wantFields string }{
		{`(class {x:Integer y:String}) TypeUtil.pick [x/q]`, "object<", "{x:Integer}"},
		{`(class {x:Integer y:String}) TypeUtil.omit [y/q]`, "object<", "{x:Integer}"},
		{`(class {x:Integer}) TypeUtil.required`, "object<", "{x:Integer}"},
		{`(class {x:Integer}) TypeUtil.merge (class {y:String})`, "object<", "{x:Integer y:String}"},
		// Overlapping fields unify (Integer within Number → Integer).
		{`(class {x:Integer}) TypeUtil.merge (class {x:Number})`, "object<", "{x:Integer}"},
	}
	for _, c := range cases {
		got := runType(t, c.expr)
		if len(got) != 1 {
			t.Fatalf("%s: got %d results", c.expr, len(got))
		}
		s := got[0].String()
		if !strings.HasPrefix(s, c.wantPrefix) || !strings.HasSuffix(s, c.wantFields) {
			t.Errorf("%s = %q, want object type with fields %s", c.expr, s, c.wantFields)
		}
	}
	// Negative: an un-unifiable field overlap is declined, for classes and
	// records alike.
	runTypeErr(t, `(class {x:Integer}) TypeUtil.merge (class {x:String})`, `field "x" cannot unify`)
	runTypeErr(t, `(refine Record [x:Integer]) TypeUtil.merge (refine Record [x:String])`, `field "x" cannot unify`)
}

// TestTypeParentOfClassWave3 pins latticeNode's non-bare branch: a class
// type value (concrete type body, not a bare lattice node) resolves its
// lattice position via Parent, so TypeUtil.parent reports Class.
func TestTypeParentOfClassWave3(t *testing.T) {
	got := runType(t, `def C (class {x:Integer}) TypeUtil.parent C`)
	if len(got) != 1 || got[0].String() != "Class" {
		t.Errorf("TypeUtil.parent (class …) = %v, want Class", formatResults(got))
	}
}

// TestTypeFnSigIntrospectionWave3 pins the FnUndefInfo branch of fnSigs:
// a body-less fnsig value answers paramsof / returnsof / arityof, with
// single and multiple declared returns; a named fn's declared return is
// reported precisely; a non-function is declined.
func TestTypeFnSigIntrospectionWave3(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`(fnsig [[Integer] [String]]) TypeUtil.paramsof`, "[Integer]"},
		{`(fnsig [[Integer String] [Boolean]]) TypeUtil.paramsof`, "[Integer String]"},
		{`(fnsig [[Integer] [String]]) TypeUtil.returnsof`, "String"},
		{`(fnsig [[Integer] [Integer String]]) TypeUtil.returnsof`, "[Integer String]"},
		{`(fnsig [[Integer] [String]]) TypeUtil.arityof`, "1"},
		// A with-body fn's declared return, and an optional param dropping
		// out of the arity count.
		{`(fn [[x:Integer] [Integer] [x]]) TypeUtil.returnsof`, "Integer"},
		{`([x:Integer y?:Integer] => [x]) TypeUtil.arityof`, "1"},
	}
	for _, c := range cases {
		got := runType(t, c.expr)
		if len(got) != 1 || got[0].String() != c.want {
			t.Errorf("%s = %v, want %q", c.expr, formatResults(got), c.want)
		}
	}
	runTypeErr(t, `5 TypeUtil.returnsof`, "expected a Function or FunctionSignature")
	runTypeErr(t, `5 TypeUtil.paramsof`, "expected a Function or FunctionSignature")
	runTypeErr(t, `5 TypeUtil.arityof`, "expected a Function or FunctionSignature")
}

// TestTypeFieldNamesWave3 pins fieldNames' decoding edges: a non-atom /
// non-string element and a non-concrete list are declined through the
// word surface; an exactly-String-typed element (only constructible at
// the host level — parser strings are Proper/Empty subtypes) is accepted
// by the String branch.
func TestTypeFieldNamesWave3(t *testing.T) {
	runTypeErr(t, `(refine Record [x:Integer]) TypeUtil.pick [1]`, "field-name element must be Atom or String")
	runTypeErr(t, `(refine Record [x:Integer]) TypeUtil.omit [1]`, "field-name element must be Atom or String")
	runTypeErr(t, `(refine Record [x:Integer]) TypeUtil.pick List`, "expected a concrete list of field names")

	r := typeRegistry(t)
	list := native.NewList([]native.Value{
		native.NewValueRaw(native.TString, native.StrPayload{S: "x"}),
		native.NewAtom("y"),
	})
	names, err := fieldNames(list, "wave3", r)
	if err != nil {
		t.Fatalf("fieldNames String+Atom: %v", err)
	}
	if len(names) != 2 || names[0] != "x" || names[1] != "y" {
		t.Errorf("fieldNames = %v, want [x y]", names)
	}
}

// TestTypeRequiredDisjunctEdgesWave3 pins stripNoneFromField: a 3-alt
// field collapses to a 2-alt disjunct (the NewDisjunct arm); an all-None
// disjunct — not constructible from boru, where `None tor None` collapses
// to None — is returned unchanged (kept==0 arm), never emptied.
func TestTypeRequiredDisjunctEdgesWave3(t *testing.T) {
	got := runType(t, `(refine Record [y:[String tor Integer tor None]]) TypeUtil.required`)
	if len(got) != 1 || got[0].String() != "record{y:Integer tor String}" {
		t.Errorf("required 3-alt = %v, want record{y:Integer tor String}", formatResults(got))
	}
	// Direct: the all-None disjunct keeps its shape.
	d := native.NewDisjunct([]native.Value{native.NewNone(), native.NewNone()})
	if !native.IsDisjunct(d) {
		t.Fatal("hand-built all-None disjunct did not stay a disjunct")
	}
	out := stripNoneFromField(d)
	if !native.IsDisjunct(out) {
		t.Errorf("stripNoneFromField(all-None) = %s, want the input disjunct back", out.String())
	}
	// And a non-disjunct field is returned as-is.
	iv := native.NewTypeLiteral(native.TInteger)
	if got := stripNoneFromField(iv); got.String() != "Integer" {
		t.Errorf("stripNoneFromField(Integer) = %s, want Integer", got.String())
	}
}

// TestTypeExcludeStructuralWave3 pins altSubtypes' structural fallback:
// record bodies are compared by value equality (not lattice ancestry), so
// exclude removes the identical record and extract keeps it.
func TestTypeExcludeStructuralWave3(t *testing.T) {
	got := runType(t, `((refine Record [x:Integer]) tor String) TypeUtil.exclude (refine Record [x:Integer])`)
	if len(got) != 1 || got[0].String() != "String" {
		t.Errorf("structural exclude = %v, want String", formatResults(got))
	}
	got = runType(t, `((refine Record [x:Integer]) tor String) TypeUtil.extract (refine Record [x:Integer])`)
	if len(got) != 1 || got[0].String() != "record{x:Integer}" {
		t.Errorf("structural extract = %v, want record{x:Integer}", formatResults(got))
	}
	// A different record body is NOT excluded (equality, not name-match).
	got = runType(t, `((refine Record [x:Integer]) tor String) TypeUtil.exclude (refine Record [x:String])`)
	if len(got) != 1 || got[0].String() != "String tor record{x:Integer}" {
		t.Errorf("structural exclude non-equal = %v, want both alts kept", formatResults(got))
	}
}
