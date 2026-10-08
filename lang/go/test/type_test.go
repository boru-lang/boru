package test

import (
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// Tests for the uniform type constructor `type` — see
// design/legacy/TYPE-UNIFORM.10.ignore. `refine BaseType arg` constructs
// a type from a base type plus an argument, and is paired with `def`:
//
//	def Acct (type Object {x:Integer})
//
// `def` is the universal binder; a capitalised name is a type binding.
//
// These tests assert the constructor works.

// type Record [a:T …] builds a record type from a list of field
// pairs; make instantiates it.
func TestTypeRecord(t *testing.T) {
	got := runOne(t, "def Pt (refine Record [x:Integer y:Integer])\n( make Pt {x:3 y:4} ) .x")
	if len(got) != 1 || got[0] != int64(3) {
		t.Errorf("got %v, want [3]", got)
	}
}

// A record takes a list, not a map — `refine Record {…}` is rejected.
func TestTypeRecordRejectsMap(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	if _, err := a.Run("refine Record {x:Integer}"); err == nil {
		t.Fatal("expected error for `refine Record {x:Integer}` (map), got nil")
	}
}

// A type-built record renders as a record type.
func TestTypeRecordRenders(t *testing.T) {
	// The name denotes its NODE (the Stage 2 flip); the record schema
	// is the node's recorded content.
	got := runOne(t, "def Pt (refine Record [x:Integer y:String])\nPt")
	if len(got) != 1 || got[0] != "Pt" {
		t.Errorf("got %v, want [Pt]", got)
	}
}

// type Table <recordtype> builds a table type; make builds rows.
func TestTypeTable(t *testing.T) {
	got := runOne(t, "def Row (refine Record [n:String])\n"+
		"def Tbl (refine Table Row)\n"+
		"make Tbl [[\"a\"] [\"b\"] [\"c\"]] size")
	if len(got) != 1 || got[0] != int64(3) {
		t.Errorf("got %v, want [3]", got)
	}
}

// type Object {fields} builds an object type; make + .field works.
func TestTypeObject(t *testing.T) {
	got := runOne(t, "def Acct (class {bal:Number})\n( make Acct {bal:50} ) .bal")
	if len(got) != 1 || got[0] != int64(50) {
		t.Errorf("got %v, want [50]", got)
	}
}

// Inheritance: applying an existing object type extends it. The child
// instance carries both the parent's and its own fields.
func TestTypeObjectInheritance(t *testing.T) {
	got := runOne(t, "def Animal (class {legs:Integer})\n"+
		"def Dog (refine Animal {breed:String})\n"+
		"( make Dog {legs:4 breed:\"lab\"} ) .legs")
	if len(got) != 1 || got[0] != int64(4) {
		t.Errorf("got %v, want [4]", got)
	}
}

// A child instance satisfies the parent type via `is`.
func TestTypeObjectInheritanceIsCheck(t *testing.T) {
	// `is` yields a boolean; (*Boru).Run stringifies non-int/string
	// results, so the boolean surfaces as the string "true".
	got := runOne(t, "def Animal (class {legs:Integer})\n"+
		"def Dog (refine Animal {breed:String})\n"+
		"make Dog {legs:4 breed:\"lab\"} is Animal")
	if len(got) != 1 || got[0] != "true" {
		t.Errorf("got %v, want [true]", got)
	}
}

// type with a base that is not a structural type is rejected.
func TestTypeBadBase(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	seedBoru(a)
	if _, err := a.Run("refine Integer {x:1}"); err == nil {
		t.Fatal("expected error for `refine Integer {x:1}`, got nil")
	}
}
