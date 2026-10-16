package test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// geoModule is a boru module, defined inline and imported, that exports
// two USER-DEFINED TYPES and two functions whose signatures accept those
// types:
//
//   - Point   — a structural record type (refine Record).
//   - Celsius — a nominal scalar newtype (refine Integer).
//   - manhattan : (Point)   -> Integer
//   - freezing  : (Celsius) -> Boolean
//
// The exported functions are referenced as data with `/v`; the types are
// exported as their type literals. After import the namespace `Geo` makes
// both the types (`Geo.Point`, `Geo.Celsius`) and the functions
// (`Geo.manhattan`, `Geo.freezing`) available to the caller.
const geoModule = `import module [
  def Point   refine Record [x:Integer y:Integer]
  def Celsius refine Integer
  def manhattan fn [[p:Point]   [Integer] [(p.x) add (p.y)]]
  def freezing  fn [[c:Celsius] [Boolean] [(c) lte 0]]
  export "Geo" {Point: Point  Celsius: Celsius  manhattan: manhattan/v  freezing: freezing/v}
]
`

// TestModuleExportedTypesRejectWrongType confirms the exported type is a
// real constraint, not decoration: passing a value that does not inhabit
// the parameter's exported type is rejected by both the checker (a type
// error) and the interpreter (no signature matches). This is the negative
// half of the contract the positive test above relies on.
func TestModuleExportedTypesRejectWrongType(t *testing.T) {
	// A plain Integer is NOT a (nominal) Geo.Celsius, so `freezing` must
	// reject it everywhere.
	src := geoModule + `Geo.freezing 5`

	// Checker rejects it.
	ac, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	res, err := ac.Check(src)
	if err != nil {
		t.Fatalf("checker: %v", err)
	}
	if res.Summary.Errors == 0 {
		t.Errorf("checker accepted a plain Integer for a Celsius parameter; want a type error (stack=%v)", res.Stack)
	}

	// Interpreter rejects it: no signature matches, so the call does not
	// dispatch and surfaces an error.
	ai, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	if _, err := ai.Run(src); err == nil {
		t.Errorf("interpreter accepted a plain Integer for a Celsius parameter; want a dispatch error")
	} else if !strings.Contains(err.Error(), "signature") && !strings.Contains(err.Error(), "freezing") {
		t.Errorf("interpreter error = %v, want a signature/dispatch failure for `freezing`", err)
	}
}
